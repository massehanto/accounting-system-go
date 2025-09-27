// notification-service/main.go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "html/template"
    "net/http"
    "net/smtp"
    "os"
    "time"
    
    "github.com/gorilla/mux"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
    "github.com/massehanto/accounting-system-go/shared/service"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type NotificationService struct {
    *service.BaseService
    emailService *EmailService
}

type EmailService struct {
    SMTPHost  string
    SMTPPort  string
    Username  string
    Password  string
    templates map[string]*template.Template
}

type EmailRequest struct {
    To       string                 `json:"to"`
    Subject  string                 `json:"subject"`
    Template string                 `json:"template"`
    Data     map[string]interface{} `json:"data"`
}

type EmailResponse struct {
    Status    string    `json:"status"`
    MessageID string    `json:"message_id"`
    SentAt    time.Time `json:"sent_at"`
}

func main() {
    cfg := config.Load()
    
    emailService := &EmailService{
        SMTPHost:  getEnv("SMTP_HOST", "smtp.gmail.com"),
        SMTPPort:  getEnv("SMTP_PORT", "587"),
        Username:  os.Getenv("SMTP_USER"),
        Password:  os.Getenv("SMTP_PASSWORD"),
        templates: make(map[string]*template.Template),
    }
    
    if err := emailService.loadTemplates(); err != nil {
        panic(fmt.Sprintf("Failed to load email templates: %v", err))
    }
    
    notificationService := &NotificationService{
        BaseService:  &service.BaseService{DB: nil},
        emailService: emailService,
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(nil, "notification-service")).Methods("GET")
    r.Handle("/send-email", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.RateLimit(50),
        middleware.LoggingMiddleware,
    )(notificationService.sendEmailHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (es *EmailService) loadTemplates() error {
    templates := map[string]string{
        "invoice": `
<!DOCTYPE html>
<html>
<head><style>body{font-family:Arial,sans-serif;margin:0;padding:20px}.header{background:#1976d2;color:white;padding:20px;text-align:center}.content{padding:20px}</style></head>
<body>
<div class="header"><h1>{{.CompanyName}}</h1></div>
<div class="content">
<h2>Invoice {{.InvoiceNumber}}</h2>
<p>Dear {{.CustomerName}},</p>
<p>Please find your invoice details below:</p>
<p><strong>Invoice Number:</strong> {{.InvoiceNumber}}<br>
<strong>Date:</strong> {{.InvoiceDate}}<br>
<strong>Due Date:</strong> {{.DueDate}}<br>
<strong>Amount:</strong> {{.TotalAmount}}</p>
<p>Please ensure payment by the due date.</p>
</div>
</body>
</html>`,
        "payment_reminder": `
<!DOCTYPE html>
<html>
<head><style>body{font-family:Arial,sans-serif;margin:0;padding:20px}</style></head>
<body>
<h2>Payment Reminder</h2>
<p>Dear {{.CustomerName}},</p>
<p>This is a friendly reminder that invoice {{.InvoiceNumber}} for {{.TotalAmount}} is due on {{.DueDate}}.</p>
<p>Please process payment at your earliest convenience.</p>
</body>
</html>`,
    }
    
    for name, tmplStr := range templates {
        tmpl, err := template.New(name).Parse(tmplStr)
        if err != nil {
            return fmt.Errorf("failed to parse template %s: %v", name, err)
        }
        es.templates[name] = tmpl
    }
    
    return nil
}

func (ns *NotificationService) sendEmailHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    var req EmailRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        ns.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    validator.Required("to", req.To)
    validator.Email("to", req.To)
    validator.Required("subject", req.Subject)
    
    if !validator.IsValid() {
        ns.RespondValidationError(w, validator.Errors())
        return
    }
    
    var body string
    var err error
    
    if req.Template != "" && ns.emailService.templates[req.Template] != nil {
        body, err = ns.emailService.renderTemplate(req.Template, req.Data)
        if err != nil {
            ns.RespondWithError(w, http.StatusInternalServerError, "TEMPLATE_ERROR", "Error rendering template")
            return
        }
    } else if req.Data["message"] != nil {
        body = req.Data["message"].(string)
    } else {
        ns.RespondWithError(w, http.StatusBadRequest, "MISSING_CONTENT", "Email content or template required")
        return
    }
    
    messageID, err := ns.emailService.SendEmailWithContext(ctx, req.To, req.Subject, body)
    if err != nil {
        ns.RespondWithError(w, http.StatusInternalServerError, "EMAIL_ERROR", fmt.Sprintf("Failed to send email: %v", err))
        return
    }
    
    response := EmailResponse{
        Status:    "sent",
        MessageID: messageID,
        SentAt:    time.Now(),
    }
    
    ns.RespondWithJSON(w, http.StatusOK, response)
}

func (es *EmailService) SendEmailWithContext(ctx context.Context, to, subject, body string) (string, error) {
    if es.Username == "" || es.Password == "" {
        return "", fmt.Errorf("SMTP credentials not configured")
    }
    
    messageID := fmt.Sprintf("<%d@accounting-system>", time.Now().UnixNano())
    
    headers := map[string]string{
        "From":         es.Username,
        "To":           to,
        "Subject":      subject,
        "Message-ID":   messageID,
        "Date":         time.Now().Format(time.RFC1123Z),
        "Content-Type": "text/html; charset=UTF-8",
    }
    
    var msg bytes.Buffer
    for key, value := range headers {
        msg.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
    }
    msg.WriteString("\r\n")
    msg.WriteString(body)
    
    auth := smtp.PlainAuth("", es.Username, es.Password, es.SMTPHost)
    
    done := make(chan error, 1)
    go func() {
        done <- smtp.SendMail(es.SMTPHost+":"+es.SMTPPort, auth, es.Username, []string{to}, msg.Bytes())
    }()
    
    select {
    case err := <-done:
        if err != nil {
            return "", err
        }
        return messageID, nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}

func (es *EmailService) renderTemplate(templateName string, data map[string]interface{}) (string, error) {
    tmpl, exists := es.templates[templateName]
    if !exists {
        return "", fmt.Errorf("template %s not found", templateName)
    }
    
    var body bytes.Buffer
    err := tmpl.Execute(&body, data)
    return body.String(), err
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}