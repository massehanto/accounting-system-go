// transaction-service/main.go - DECOUPLED VERSION
package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"
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

type TransactionService struct {
    *service.BaseService
}

type JournalEntry struct {
    ID          int                `json:"id"`
    CompanyID   int                `json:"company_id"`
    EntryNumber string             `json:"entry_number"`
    EntryDate   time.Time          `json:"entry_date"`
    Description string             `json:"description"`
    TotalAmount float64            `json:"total_amount"`
    Status      string             `json:"status"`
    CreatedBy   int                `json:"created_by"`
    PostedBy    *int               `json:"posted_by,omitempty"`
    PostedAt    *time.Time         `json:"posted_at,omitempty"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
    Lines       []JournalEntryLine `json:"lines,omitempty"`
}

type JournalEntryLine struct {
    ID              int     `json:"id"`
    JournalEntryID  int     `json:"journal_entry_id"`
    AccountID       int     `json:"account_id"`
    Description     string  `json:"description"`
    DebitAmount     float64 `json:"debit_amount"`
    CreditAmount    float64 `json:"credit_amount"`
    CreatedAt       time.Time `json:"created_at"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "transaction_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    transactionService := &TransactionService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(db, "transaction-service")).Methods("GET")
    
    authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)
    r.Handle("/transactions", authMiddleware(transactionService.getTransactionsHandler)).Methods("GET")
    r.Handle("/transactions", authMiddleware(transactionService.createTransactionHandler)).Methods("POST")
    r.Handle("/transactions/{id}", authMiddleware(transactionService.getTransactionHandler)).Methods("GET")
    r.Handle("/transactions/{id}/post", authMiddleware(transactionService.postTransactionHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *TransactionService) getTransactionsHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID required")
        return
    }
    
    status := r.URL.Query().Get("status")
    
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    query := `SELECT id, company_id, entry_number, entry_date, description, total_amount, 
                     status, created_by, posted_by, posted_at, created_at, updated_at
              FROM journal_entries WHERE company_id = $1`
    
    args := []interface{}{companyID}
    
    if status != "" {
        query += " AND status = $2"
        args = append(args, status)
    }
    
    query += " ORDER BY created_at DESC LIMIT 50"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.HandleDBError(w, err, "Error fetching transactions")
        return
    }
    defer rows.Close()
    
    var transactions []JournalEntry
    for rows.Next() {
        var transaction JournalEntry
        var postedBy sql.NullInt64
        var postedAt sql.NullTime
        
        err := rows.Scan(&transaction.ID, &transaction.CompanyID, &transaction.EntryNumber,
                        &transaction.EntryDate, &transaction.Description, &transaction.TotalAmount,
                        &transaction.Status, &transaction.CreatedBy, &postedBy, &postedAt,
                        &transaction.CreatedAt, &transaction.UpdatedAt)
        if err != nil {
            continue
        }
        
        if postedBy.Valid {
            pb := int(postedBy.Int64)
            transaction.PostedBy = &pb
        }
        if postedAt.Valid {
            transaction.PostedAt = &postedAt.Time
        }
        
        transactions = append(transactions, transaction)
    }
    
    s.RespondWithJSON(w, http.StatusOK, transactions)
}

func (s *TransactionService) createTransactionHandler(w http.ResponseWriter, r *http.Request) {
    var entry JournalEntry
    if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("entry_number", entry.EntryNumber)
    validator.Required("description", entry.Description)
    
    if len(entry.Lines) < 2 {
        validator.AddError("lines", "At least two journal lines required")
    }

    var totalDebits, totalCredits float64
    for i, line := range entry.Lines {
        if line.AccountID == 0 {
            validator.AddError(fmt.Sprintf("lines[%d].account_id", i), "Account ID required")
        }
        
        if line.DebitAmount < 0 || line.CreditAmount < 0 {
            validator.AddError(fmt.Sprintf("lines[%d].amounts", i), "Amounts cannot be negative")
        }
        if line.DebitAmount > 0 && line.CreditAmount > 0 {
            validator.AddError(fmt.Sprintf("lines[%d].amounts", i), "Cannot have both debit and credit")
        }
        if line.DebitAmount == 0 && line.CreditAmount == 0 {
            validator.AddError(fmt.Sprintf("lines[%d].amounts", i), "Must have debit or credit amount")
        }
        
        totalDebits += line.DebitAmount
        totalCredits += line.CreditAmount
    }

    if abs(totalDebits-totalCredits) > 0.01 {
        validator.AddError("balance", "Total debits must equal total credits")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    entry.CompanyID = s.GetCompanyIDFromRequest(r)
    entry.CreatedBy = s.GetUserIDFromRequest(r)
    entry.Status = "draft"
    entry.TotalAmount = totalDebits

    if entry.EntryDate.IsZero() {
        entry.EntryDate = time.Now()
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check duplicate entry number
        var exists bool
        err := tx.QueryRow(
            "SELECT EXISTS(SELECT 1 FROM journal_entries WHERE company_id = $1 AND entry_number = $2)",
            entry.CompanyID, entry.EntryNumber).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "DUPLICATE_ENTRY", "Entry number exists")
            return nil
        }

        // Create journal entry
        entryQuery := `INSERT INTO journal_entries (company_id, entry_number, entry_date, description, 
                                                    total_amount, status, created_by) 
                       VALUES ($1, $2, $3, $4, $5, $6, $7) 
                       RETURNING id, created_at, updated_at`
        
        err = tx.QueryRow(entryQuery, entry.CompanyID, entry.EntryNumber, entry.EntryDate,
                         entry.Description, entry.TotalAmount, entry.Status, entry.CreatedBy).Scan(
                         &entry.ID, &entry.CreatedAt, &entry.UpdatedAt)
        if err != nil {
            return err
        }

        // Create journal entry lines
        for i := range entry.Lines {
            entry.Lines[i].JournalEntryID = entry.ID
            lineQuery := `INSERT INTO journal_entry_lines (journal_entry_id, account_id, description, 
                                                           debit_amount, credit_amount) 
                          VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`
            
            err = tx.QueryRow(lineQuery, entry.Lines[i].JournalEntryID, entry.Lines[i].AccountID,
                             entry.Lines[i].Description, entry.Lines[i].DebitAmount, 
                             entry.Lines[i].CreditAmount).Scan(&entry.Lines[i].ID, &entry.Lines[i].CreatedAt)
            if err != nil {
                return err
            }
        }

        s.RespondWithJSON(w, http.StatusCreated, entry)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Transaction creation failed")
    }
}

func (s *TransactionService) postTransactionHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)
    userID := s.GetUserIDFromRequest(r)

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Get transaction
        var status string
        err := tx.QueryRow("SELECT status FROM journal_entries WHERE id = $1 AND company_id = $2", 
                          id, companyID).Scan(&status)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
            return nil
        }
        if err != nil {
            return err
        }
        
        if status != "draft" {
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_STATUS", "Can only post draft transactions")
            return nil
        }
        
        // Update status to posted
        now := time.Now()
        updateQuery := `UPDATE journal_entries 
                        SET status = 'posted', posted_by = $1, posted_at = $2, updated_at = CURRENT_TIMESTAMP 
                        WHERE id = $3`
        
        _, err = tx.Exec(updateQuery, userID, now, id)
        if err != nil {
            return err
        }
        
        // TODO: Publish event for ledger posting instead of direct HTTP call
        // This should be handled by an event bus (Redis/RabbitMQ)
        
        response := map[string]interface{}{
            "status":    "posted",
            "posted_at": now,
            "message":   "Transaction posted successfully",
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "POST_ERROR", "Transaction posting failed")
    }
}

func (s *TransactionService) getTransactionHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    var entry JournalEntry
    query := `SELECT id, company_id, entry_number, entry_date, description, total_amount, 
                     status, created_by, posted_by, posted_at, created_at, updated_at
              FROM journal_entries WHERE id = $1 AND company_id = $2`
    
    var postedBy sql.NullInt64
    var postedAt sql.NullTime
    
    err = s.DB.QueryRowContext(ctx, query, id, companyID).Scan(
        &entry.ID, &entry.CompanyID, &entry.EntryNumber, &entry.EntryDate,
        &entry.Description, &entry.TotalAmount, &entry.Status, &entry.CreatedBy,
        &postedBy, &postedAt, &entry.CreatedAt, &entry.UpdatedAt)
    
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Error fetching transaction")
        return
    }
    
    if postedBy.Valid {
        pb := int(postedBy.Int64)
        entry.PostedBy = &pb
    }
    if postedAt.Valid {
        entry.PostedAt = &postedAt.Time
    }
    
    // Get transaction lines
    linesQuery := `SELECT id, journal_entry_id, account_id, description, 
                          debit_amount, credit_amount, created_at
                   FROM journal_entry_lines 
                   WHERE journal_entry_id = $1 ORDER BY id`
    
    rows, err := s.DB.QueryContext(ctx, linesQuery, id)
    if err != nil {
        s.HandleDBError(w, err, "Error fetching transaction lines")
        return
    }
    defer rows.Close()
    
    for rows.Next() {
        var line JournalEntryLine
        
        err := rows.Scan(&line.ID, &line.JournalEntryID, &line.AccountID,
                        &line.Description, &line.DebitAmount, &line.CreditAmount, &line.CreatedAt)
        if err != nil {
            continue
        }
        
        entry.Lines = append(entry.Lines, line)
    }
    
    s.RespondWithJSON(w, http.StatusOK, entry)
}

func abs(x float64) float64 {
    if x < 0 {
        return -x
    }
    return x
}