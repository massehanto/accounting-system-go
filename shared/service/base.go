// shared/service/base.go - SIMPLIFIED VERSION
package service

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "strconv"
    "time"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type BaseService struct {
    DB *sql.DB
}

type ErrorResponse struct {
    Error     string    `json:"error"`
    Code      string    `json:"code,omitempty"`
    Timestamp time.Time `json:"timestamp"`
}

func (s *BaseService) RespondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    
    response := map[string]interface{}{
        "data": data,
        "timestamp": time.Now(),
    }
    
    json.NewEncoder(w).Encode(response)
}

func (s *BaseService) RespondWithError(w http.ResponseWriter, statusCode int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    
    response := ErrorResponse{
        Error:     message,
        Code:      code,
        Timestamp: time.Now(),
    }
    
    json.NewEncoder(w).Encode(response)
}

func (s *BaseService) RespondValidationError(w http.ResponseWriter, errors []validation.ValidationError) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusBadRequest)
    
    response := map[string]interface{}{
        "error":   "Validation failed",
        "details": errors,
        "timestamp": time.Now(),
    }
    
    json.NewEncoder(w).Encode(response)
}

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

func (s *BaseService) HandleDBError(w http.ResponseWriter, err error, message string) {
    s.RespondWithError(w, http.StatusInternalServerError, "DATABASE_ERROR", message)
}

func (s *BaseService) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
    tx, err := s.DB.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    
    defer func() {
        if p := recover(); p != nil {
            tx.Rollback()
            panic(p)
        } else if err != nil {
            tx.Rollback()
        } else {
            err = tx.Commit()
        }
    }()
    
    err = fn(tx)
    return err
}