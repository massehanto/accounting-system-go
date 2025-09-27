package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "time"
    
    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/database"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
    "github.com/massehanto/accounting-system-go/shared/service"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type InvoiceService struct {
    *service.BaseService
}

type Invoice struct {
    ID            int           `json:"id"`
    CompanyID     int           `json:"company_id"`
    CustomerID    int           `json:"customer_id"`
    InvoiceNumber string        `json:"invoice_number"`
    InvoiceDate   time.Time     `json:"invoice_date"`
    DueDate       time.Time     `json:"due_date"`
    Subtotal      float64       `json:"subtotal"`
    TaxAmount     float64       `json:"tax_amount"`
    TotalAmount   float64       `json:"total_amount"`
    Status        string        `json:"status"`
    CreatedAt     time.Time     `json:"created_at"`
    Customer      *Customer     `json:"customer,omitempty"`
    Lines         []InvoiceLine `json:"lines,omitempty"`
}

type Customer struct {
    ID           int    `json:"id"`
    CompanyID    int    `json:"company_id"`
    CustomerCode string `json:"customer_code"`
    Name         string `json:"name"`
    Email        string `json:"email"`
    Phone        string `json:"phone"`
    Address      string `json:"address"`
    TaxID        string `json:"tax_id"`
}

type InvoiceLine struct {
    ID          int     `json:"id"`
    InvoiceID   int     `json:"invoice_id"`
    ProductName string  `json:"product_name"`
    Quantity    float64 `json:"quantity"`
    UnitPrice   float64 `json:"unit_price"`
    LineTotal   float64 `json:"line_total"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "invoice_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    invoiceService := &InvoiceService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    api := middleware.APIMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(db, "invoice-service")).Methods("GET")
    r.Handle("/invoices", api(invoiceService.getInvoicesHandler)).Methods("GET")
    r.Handle("/invoices", api(invoiceService.createInvoiceHandler)).Methods("POST")
    r.Handle("/invoices/{id}/send", api(invoiceService.sendInvoiceHandler)).Methods("POST")
    r.Handle("/customers", api(invoiceService.getCustomersHandler)).Methods("GET")
    r.Handle("/customers", api(invoiceService.createCustomerHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *InvoiceService) getInvoicesHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `SELECT i.id, i.company_id, i.customer_id, i.invoice_number, i.invoice_date, i.due_date, 
                     i.subtotal, i.tax_amount, i.total_amount, i.status, i.created_at, c.name
              FROM invoices i LEFT JOIN customers c ON i.customer_id = c.id 
              WHERE i.company_id = $1 ORDER BY i.created_at DESC`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching invoices")
        return
    }
    defer rows.Close()
    
    var invoices []Invoice
    for rows.Next() {
        var invoice Invoice
        var customerName sql.NullString
        err := rows.Scan(&invoice.ID, &invoice.CompanyID, &invoice.CustomerID, &invoice.InvoiceNumber,
                        &invoice.InvoiceDate, &invoice.DueDate, &invoice.Subtotal, &invoice.TaxAmount,
                        &invoice.TotalAmount, &invoice.Status, &invoice.CreatedAt, &customerName)
        if err != nil {
            continue
        }
        if customerName.Valid {
            invoice.Customer = &Customer{Name: customerName.String}
        }
        invoices = append(invoices, invoice)
    }
    
    s.RespondWithJSON(w, http.StatusOK, invoices)
}

func (s *InvoiceService) getCustomersHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `SELECT id, company_id, customer_code, name, email, phone, address, tax_id
              FROM customers WHERE company_id = $1 ORDER BY name`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching customers")
        return
    }
    defer rows.Close()
    
    var customers []Customer
    for rows.Next() {
        var customer Customer
        err := rows.Scan(&customer.ID, &customer.CompanyID, &customer.CustomerCode, &customer.Name,
                        &customer.Email, &customer.Phone, &customer.Address, &customer.TaxID)
        if err != nil {
            continue
        }
        customers = append(customers, customer)
    }
    
    s.RespondWithJSON(w, http.StatusOK, customers)
}

func (s *InvoiceService) createInvoiceHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    var invoice Invoice
    if err := json.NewDecoder(r.Body).Decode(&invoice); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("invoice_number", invoice.InvoiceNumber)
    
    if invoice.CustomerID == 0 {
        validator.AddError("customer_id", "Customer ID is required")
    }
    
    if len(invoice.Lines) == 0 {
        validator.AddError("lines", "At least one invoice line is required")
    }

    var subtotal float64
    for i, line := range invoice.Lines {
        validator.Required(fmt.Sprintf("lines[%d].product_name", i), line.ProductName)
        if line.Quantity <= 0 {
            validator.AddError(fmt.Sprintf("lines[%d].quantity", i), "Quantity must be positive")
        }
        if line.UnitPrice < 0 {
            validator.AddError(fmt.Sprintf("lines[%d].unit_price", i), "Unit price cannot be negative")
        }
        
        expectedTotal := line.Quantity * line.UnitPrice
        if abs(line.LineTotal-expectedTotal) > 0.01 {
            validator.AddError(fmt.Sprintf("lines[%d].line_total", i), "Line total calculation incorrect")
        }
        subtotal += line.LineTotal
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    invoice.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    invoice.Subtotal = subtotal
    invoice.TaxAmount = subtotal * 0.11
    invoice.TotalAmount = subtotal + invoice.TaxAmount
    invoice.Status = "draft"

    tx, err := s.DB.BeginTx(ctx, nil)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Transaction failed")
        return
    }
    defer tx.Rollback()

    query := `INSERT INTO invoices (company_id, customer_id, invoice_number, invoice_date, due_date, subtotal, tax_amount, total_amount, status) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
              RETURNING id, created_at`
    
    err = tx.QueryRowContext(ctx, query, 
        invoice.CompanyID, invoice.CustomerID, invoice.InvoiceNumber,
        invoice.InvoiceDate, invoice.DueDate, invoice.Subtotal, 
        invoice.TaxAmount, invoice.TotalAmount, invoice.Status).Scan(&invoice.ID, &invoice.CreatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating invoice")
        return
    }

    for i := range invoice.Lines {
        invoice.Lines[i].InvoiceID = invoice.ID
        lineQuery := `INSERT INTO invoice_lines (invoice_id, product_name, quantity, unit_price, line_total) 
                      VALUES ($1, $2, $3, $4, $5) RETURNING id`
        
        err = tx.QueryRowContext(ctx, lineQuery, 
            invoice.Lines[i].InvoiceID, invoice.Lines[i].ProductName, 
            invoice.Lines[i].Quantity, invoice.Lines[i].UnitPrice, 
            invoice.Lines[i].LineTotal).Scan(&invoice.Lines[i].ID)
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error creating invoice lines")
            return
        }
    }

    if err = tx.Commit(); err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "COMMIT_ERROR", "Failed to commit")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, invoice)
}

func (s *InvoiceService) createCustomerHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    var customer Customer
    if err := json.NewDecoder(r.Body).Decode(&customer); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("customer_code", customer.CustomerCode)
    validator.Required("name", customer.Name)
    validator.Email("email", customer.Email)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    customer.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))

    query := `INSERT INTO customers (company_id, customer_code, name, email, phone, address, tax_id) 
              VALUES ($1, $2, $3, $4, $5, $6, $7) 
              RETURNING id`
    
    err := s.DB.QueryRowContext(ctx, query, customer.CompanyID, customer.CustomerCode, customer.Name,
                               customer.Email, customer.Phone, customer.Address, customer.TaxID).Scan(&customer.ID)
    if err != nil {
        s.HandleDBError(w, err, "Error creating customer")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, customer)
}

func (s *InvoiceService) sendInvoiceHandler(w http.ResponseWriter, r *http.Request) {
    s.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func abs(x float64) float64 {
    if x < 0 {
        return -x
    }
    return x
}