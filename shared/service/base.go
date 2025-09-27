// shared/service/base.go - ENHANCED VERSION
package service

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"
    
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type BaseService struct {
    DB *sql.DB
}

type ErrorResponse struct {
    Error       string      `json:"error"`
    Code        string      `json:"code,omitempty"`
    Details     interface{} `json:"details,omitempty"`
    Timestamp   time.Time   `json:"timestamp"`
    RequestID   string      `json:"request_id,omitempty"`
    TraceID     string      `json:"trace_id,omitempty"`
    UserMessage string      `json:"user_message,omitempty"`
}

type SuccessResponse struct {
    Data      interface{} `json:"data"`
    Status    string      `json:"status"`
    Timestamp time.Time   `json:"timestamp"`
    RequestID string      `json:"request_id,omitempty"`
    Message   string      `json:"message,omitempty"`
}

type PaginatedResponse struct {
    Data       interface{} `json:"data"`
    TotalCount int         `json:"total_count"`
    Page       int         `json:"page"`
    PageSize   int         `json:"page_size"`
    HasNext    bool        `json:"has_next"`
    HasPrev    bool        `json:"has_prev"`
    Links      PageLinks   `json:"links,omitempty"`
}

type PageLinks struct {
    First    string `json:"first,omitempty"`
    Previous string `json:"previous,omitempty"`
    Next     string `json:"next,omitempty"`
    Last     string `json:"last,omitempty"`
}

type TransactionFunc func(*sql.Tx) error

// Enhanced response helpers with Indonesian localization
func (s *BaseService) RespondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Header().Set("X-Content-Language", "id-ID")
    w.Header().Set("X-Currency", "IDR")
    w.Header().Set("X-Timezone", "Asia/Jakarta")
    w.WriteHeader(statusCode)
    
    response := SuccessResponse{
        Data:      s.formatIndonesianData(data),
        Status:    "success",
        Timestamp: time.Now().In(time.FixedZone("WIB", 7*3600)), // Jakarta timezone
        RequestID: w.Header().Get("X-Request-ID"),
    }
    
    if err := json.NewEncoder(w).Encode(response); err != nil {
        log.Printf("Error encoding JSON response: %v", err)
    }
}

func (s *BaseService) RespondWithError(w http.ResponseWriter, statusCode int, code, message string) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Header().Set("X-Content-Language", "id-ID")
    w.WriteHeader(statusCode)
    
    response := ErrorResponse{
        Error:       message,
        Code:        code,
        Timestamp:   time.Now().In(time.FixedZone("WIB", 7*3600)),
        RequestID:   w.Header().Get("X-Request-ID"),
        TraceID:     w.Header().Get("X-Trace-ID"),
        UserMessage: s.getIndonesianErrorMessage(code, message),
    }
    
    if err := json.NewEncoder(w).Encode(response); err != nil {
        log.Printf("Error encoding error response: %v", err)
    }
}

func (s *BaseService) RespondValidationError(w http.ResponseWriter, errors []validation.ValidationError) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.Header().Set("X-Content-Language", "id-ID")
    w.WriteHeader(http.StatusBadRequest)
    
    response := ErrorResponse{
        Error:       "Validation failed",
        Code:        "VALIDATION_ERROR",
        Details:     s.translateValidationErrors(errors),
        Timestamp:   time.Now().In(time.FixedZone("WIB", 7*3600)),
        RequestID:   w.Header().Get("X-Request-ID"),
        UserMessage: "Data yang dimasukkan tidak valid. Silakan periksa kembali.",
    }
    
    json.NewEncoder(w).Encode(response)
}

func (s *BaseService) RespondWithPagination(w http.ResponseWriter, data interface{}, totalCount, page, pageSize int, baseURL string) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    
    if page < 1 {
        page = 1
    }
    if pageSize < 1 {
        pageSize = 20
    }
    
    totalPages := (totalCount + pageSize - 1) / pageSize
    
    response := PaginatedResponse{
        Data:       s.formatIndonesianData(data),
        TotalCount: totalCount,
        Page:       page,
        PageSize:   pageSize,
        HasNext:    page < totalPages,
        HasPrev:    page > 1,
        Links:      s.buildPageLinks(baseURL, page, pageSize, totalPages),
    }
    
    json.NewEncoder(w).Encode(response)
}

// Enhanced transaction helper with retry logic
func (s *BaseService) WithTransaction(ctx context.Context, fn TransactionFunc) error {
    return s.WithTransactionRetry(ctx, fn, 3)
}

func (s *BaseService) WithTransactionRetry(ctx context.Context, fn TransactionFunc, maxRetries int) error {
    var lastErr error
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        tx, err := s.DB.BeginTx(ctx, nil)
        if err != nil {
            lastErr = fmt.Errorf("failed to begin transaction (attempt %d): %w", attempt, err)
            if attempt < maxRetries {
                time.Sleep(time.Duration(attempt) * 100 * time.Millisecond) // Exponential backoff
                continue
            }
            return lastErr
        }
        
        defer func() {
            if p := recover(); p != nil {
                tx.Rollback()
                panic(p)
            }
        }()
        
        if err := fn(tx); err != nil {
            if rbErr := tx.Rollback(); rbErr != nil {
                lastErr = fmt.Errorf("transaction error: %v, rollback error: %v", err, rbErr)
            } else {
                lastErr = err
            }
            
            // Check if error is retryable
            if attempt < maxRetries && s.isRetryableError(err) {
                time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
                continue
            }
            return lastErr
        }
        
        if err := tx.Commit(); err != nil {
            lastErr = fmt.Errorf("failed to commit transaction: %w", err)
            if attempt < maxRetries && s.isRetryableError(err) {
                time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
                continue
            }
            return lastErr
        }
        
        return nil // Success
    }
    
    return lastErr
}

// Enhanced context timeout helper with progress tracking
func (s *BaseService) ExecuteWithTimeout(timeout time.Duration, fn func(context.Context) error) error {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    done := make(chan error, 1)
    go func() {
        done <- fn(ctx)
    }()
    
    select {
    case err := <-done:
        return err
    case <-ctx.Done():
        return fmt.Errorf("operation timed out after %v", timeout)
    }
}

// Enhanced request helper methods
func (s *BaseService) GetCompanyIDFromRequest(r *http.Request) int {
    if companyIDStr := r.Header.Get("Company-ID"); companyIDStr != "" {
        if companyID, err := strconv.Atoi(companyIDStr); err == nil {
            return companyID
        }
    }
    return 0
}

func (s *BaseService) GetUserIDFromRequest(r *http.Request) int {
    if userIDStr := r.Header.Get("User-ID"); userIDStr != "" {
        if userID, err := strconv.Atoi(userIDStr); err == nil {
            return userID
        }
    }
    return 0
}

func (s *BaseService) GetUserRoleFromRequest(r *http.Request) string {
    return r.Header.Get("User-Role")
}

// Enhanced database error handling with Indonesian context
func (s *BaseService) HandleDBError(w http.ResponseWriter, err error, message string) {
    log.Printf("Database error: %v", err)
    
    errStr := strings.ToLower(err.Error())
    
    switch {
    case strings.Contains(errStr, "duplicate key"):
        s.RespondWithError(w, http.StatusConflict, "DUPLICATE_ENTRY", "Data sudah ada sebelumnya")
    case strings.Contains(errStr, "foreign key"):
        s.RespondWithError(w, http.StatusBadRequest, "REFERENCE_ERROR", "Data yang direferensikan tidak ditemukan")
    case strings.Contains(errStr, "connection"):
        s.RespondWithError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Koneksi database bermasalah")
    case strings.Contains(errStr, "timeout"):
        s.RespondWithError(w, http.StatusRequestTimeout, "DATABASE_TIMEOUT", "Operasi database melebihi batas waktu")
    case strings.Contains(errStr, "deadlock"):
        s.RespondWithError(w, http.StatusConflict, "DATABASE_DEADLOCK", "Konflik dalam pengaksesan data")
    default:
        s.RespondWithError(w, http.StatusInternalServerError, "DATABASE_ERROR", message)
    }
}

// Enhanced input validation helper
func (s *BaseService) ValidateCompanyAccess(r *http.Request, companyID int) bool {
    requestCompanyID := s.GetCompanyIDFromRequest(r)
    return requestCompanyID == companyID && companyID > 0
}

func (s *BaseService) ValidateUserPermission(r *http.Request, requiredRole string) bool {
    userRole := s.GetUserRoleFromRequest(r)
    
    roleHierarchy := map[string]int{
        "user":       1,
        "accountant": 2,
        "manager":    3,
        "admin":      4,
    }
    
    userLevel := roleHierarchy[userRole]
    requiredLevel := roleHierarchy[requiredRole]
    
    return userLevel >= requiredLevel
}

// Indonesian data formatting
func (s *BaseService) formatIndonesianData(data interface{}) interface{} {
    if data == nil {
        return nil
    }
    
    switch v := data.(type) {
    case map[string]interface{}:
        formatted := make(map[string]interface{})
        for key, value := range v {
            if s.isCurrencyField(key) {
                if num, ok := value.(float64); ok {
                    formatted[key] = s.formatIndonesianCurrency(num)
                } else {
                    formatted[key] = value
                }
            } else if s.isDateField(key) {
                if str, ok := value.(string); ok {
                    formatted[key] = s.formatIndonesianDate(str)
                } else {
                    formatted[key] = value
                }
            } else {
                formatted[key] = s.formatIndonesianData(value)
            }
        }
        return formatted
    case []interface{}:
        formatted := make([]interface{}, len(v))
        for i, item := range v {
            formatted[i] = s.formatIndonesianData(item)
        }
        return formatted
    default:
        return v
    }
}

func (s *BaseService) isCurrencyField(key string) bool {
    currencyFields := []string{
        "amount", "price", "total", "balance", "subtotal", "tax_amount",
        "total_amount", "debit_amount", "credit_amount", "unit_price",
        "cost_price", "line_total", "grand_total",
    }
    
    lowerKey := strings.ToLower(key)
    for _, field := range currencyFields {
        if strings.Contains(lowerKey, field) {
            return true
        }
    }
    return false
}

func (s *BaseService) isDateField(key string) bool {
    dateFields := []string{"date", "created_at", "updated_at", "posted_at", "last_login"}
    lowerKey := strings.ToLower(key)
    for _, field := range dateFields {
        if strings.Contains(lowerKey, field) {
            return true
        }
    }
    return false
}

func (s *BaseService) formatIndonesianCurrency(amount float64) float64 {
    // Indonesian Rupiah doesn't use decimal places
    return float64(int64(amount))
}

func (s *BaseService) formatIndonesianDate(dateStr string) string {
    if dateStr == "" {
        return dateStr
    }
    
    // Parse and format to Indonesian date format
    if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
        return t.In(time.FixedZone("WIB", 7*3600)).Format("02/01/2006")
    }
    return dateStr
}

// Indonesian error message translation
func (s *BaseService) getIndonesianErrorMessage(code, originalMessage string) string {
    translations := map[string]string{
        "VALIDATION_ERROR":    "Data tidak valid",
        "DUPLICATE_ENTRY":     "Data sudah ada sebelumnya",
        "NOT_FOUND":          "Data tidak ditemukan",
        "ACCESS_DENIED":      "Akses ditolak",
        "UNAUTHORIZED":       "Tidak memiliki otorisasi",
        "DATABASE_ERROR":     "Terjadi kesalahan pada database",
        "NETWORK_ERROR":      "Terjadi kesalahan jaringan",
        "INVALID_INPUT":      "Input tidak valid",
        "MISSING_REQUIRED":   "Data wajib tidak lengkap",
        "CURRENCY_ERROR":     "Format mata uang tidak valid",
        "TAX_CALCULATION":    "Kesalahan perhitungan pajak",
    }
    
    if indonesian, exists := translations[code]; exists {
        return indonesian
    }
    return originalMessage
}

func (s *BaseService) translateValidationErrors(errors []validation.ValidationError) []validation.ValidationError {
    translated := make([]validation.ValidationError, len(errors))
    
    for i, err := range errors {
        translated[i] = validation.ValidationError{
            Field:   err.Field,
            Message: s.translateFieldError(err.Field, err.Message),
            Code:    err.Code,
        }
    }
    
    return translated
}

func (s *BaseService) translateFieldError(field, message string) string {
    fieldTranslations := map[string]string{
        "email":           "Email",
        "password":        "Kata sandi",
        "name":           "Nama",
        "account_code":   "Kode akun",
        "account_name":   "Nama akun",
        "amount":         "Jumlah",
        "description":    "Deskripsi",
        "tax_id":         "NPWP",
        "phone":          "Nomor telepon",
        "address":        "Alamat",
    }
    
    // Translate field name if available
    if indonesian, exists := fieldTranslations[field]; exists {
        message = strings.Replace(message, field, indonesian, -1)
    }
    
    // Translate common validation messages
    messageTranslations := map[string]string{
        "is required":                    "wajib diisi",
        "must be at least":              "minimal",
        "characters":                    "karakter",
        "Invalid email format":          "Format email tidak valid",
        "must be 4 digits":             "harus 4 digit",
        "cannot be negative":           "tidak boleh negatif",
        "must be positive":             "harus bernilai positif",
    }
    
    for english, indonesian := range messageTranslations {
        message = strings.Replace(message, english, indonesian, -1)
    }
    
    return message
}

// Helper methods
func (s *BaseService) buildPageLinks(baseURL string, page, pageSize, totalPages int) PageLinks {
    links := PageLinks{}
    
    if baseURL != "" {
        if page > 1 {
            links.First = fmt.Sprintf("%s?page=1&page_size=%d", baseURL, pageSize)
            links.Previous = fmt.Sprintf("%s?page=%d&page_size=%d", baseURL, page-1, pageSize)
        }
        
        if page < totalPages {
            links.Next = fmt.Sprintf("%s?page=%d&page_size=%d", baseURL, page+1, pageSize)
            links.Last = fmt.Sprintf("%s?page=%d&page_size=%d", baseURL, totalPages, pageSize)
        }
    }
    
    return links
}

func (s *BaseService) isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    
    errStr := strings.ToLower(err.Error())
    retryableErrors := []string{
        "connection", "timeout", "deadlock", "lock", "temporary",
    }
    
    for _, retryable := range retryableErrors {
        if strings.Contains(errStr, retryable) {
            return true
        }
    }
    
    return false
}