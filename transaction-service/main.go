// transaction-service/main.go - COMPLETE CORRECTED VERSION
package main

import (
    "bytes"
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
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
    accountServiceURL string
    client           *http.Client
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
    ID              int          `json:"id"`
    JournalEntryID  int          `json:"journal_entry_id"`
    AccountID       int          `json:"account_id"`
    Description     string       `json:"description"`
    DebitAmount     float64      `json:"debit_amount"`
    CreditAmount    float64      `json:"credit_amount"`
    CreatedAt       time.Time    `json:"created_at"`
    Account         *AccountInfo `json:"account,omitempty"`
}

type AccountInfo struct {
    ID          int    `json:"id"`
    AccountCode string `json:"account_code"`
    AccountName string `json:"account_name"`
    AccountType string `json:"account_type"`
}

func main() {
    cfg := config.ValidateAndLoad()
    cfg.Database.Name = "transaction_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    transactionService := &TransactionService{
        BaseService: &service.BaseService{DB: db},
        accountServiceURL: getEnv("ACCOUNT_SERVICE_URL", "http://localhost:8002"),
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
    
    r := mux.NewRouter()
    
    // Health check
    r.Handle("/health", middleware.HealthCheck(db, "transaction-service")).Methods("GET")
    
    // Transaction endpoints
    authMiddleware := middleware.APIMiddleware(cfg.JWT.Secret)
    r.Handle("/transactions", authMiddleware(transactionService.getTransactionsHandler)).Methods("GET")
    r.Handle("/transactions", authMiddleware(transactionService.createTransactionHandler)).Methods("POST")
    r.Handle("/transactions/{id}", authMiddleware(transactionService.getTransactionHandler)).Methods("GET")
    r.Handle("/transactions/{id}", authMiddleware(transactionService.updateTransactionHandler)).Methods("PUT")
    r.Handle("/transactions/{id}/post", authMiddleware(transactionService.postTransactionHandler)).Methods("POST")
    r.Handle("/transactions/{id}/reverse", authMiddleware(transactionService.reverseTransactionHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *TransactionService) getTransactionsHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID is required")
        return
    }
    
    // Enhanced query parameters
    status := r.URL.Query().Get("status")
    startDate := r.URL.Query().Get("start_date")
    endDate := r.URL.Query().Get("end_date")
    limit := r.URL.Query().Get("limit")
    offset := r.URL.Query().Get("offset")
    
    if limit == "" {
        limit = "50"
    }
    if offset == "" {
        offset = "0"
    }

    err := s.ExecuteWithTimeout(15*time.Second, func(ctx context.Context) error {
        query := `SELECT id, company_id, entry_number, entry_date, description, total_amount, 
                         status, created_by, posted_by, posted_at, created_at, updated_at
                  FROM journal_entries WHERE company_id = $1`
        
        args := []interface{}{companyID}
        argCount := 1
        
        if status != "" {
            argCount++
            query += " AND status = $" + strconv.Itoa(argCount)
            args = append(args, status)
        }
        
        if startDate != "" {
            argCount++
            query += " AND entry_date >= $" + strconv.Itoa(argCount)
            args = append(args, startDate)
        }
        
        if endDate != "" {
            argCount++
            query += " AND entry_date <= $" + strconv.Itoa(argCount)
            args = append(args, endDate)
        }
        
        query += " ORDER BY created_at DESC"
        
        // Add pagination
        argCount++
        query += " LIMIT $" + strconv.Itoa(argCount)
        args = append(args, limit)
        
        argCount++
        query += " OFFSET $" + strconv.Itoa(argCount)
        args = append(args, offset)
        
        rows, err := s.DB.QueryContext(ctx, query, args...)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching transactions")
            return nil
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
        
        // Get total count for pagination
        countQuery := `SELECT COUNT(*) FROM journal_entries WHERE company_id = $1`
        countArgs := []interface{}{companyID}
        
        if status != "" {
            countQuery += " AND status = $2"
            countArgs = append(countArgs, status)
        }
        
        var totalCount int
        err = s.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
        if err != nil {
            totalCount = 0
        }
        
        response := map[string]interface{}{
            "data":        transactions,
            "total_count": totalCount,
            "limit":       limit,
            "offset":      offset,
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving transactions")
    }
}

func (s *TransactionService) createTransactionHandler(w http.ResponseWriter, r *http.Request) {
    var entry JournalEntry
    if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    // Enhanced validation
    validator := validation.New()
    validator.Required("entry_number", entry.EntryNumber)
    validator.MinLength("entry_number", entry.EntryNumber, 3)
    validator.MaxLength("entry_number", entry.EntryNumber, 50)
    validator.Required("description", entry.Description)
    validator.MinLength("description", entry.Description, 5)
    validator.MaxLength("description", entry.Description, 500)
    
    if len(entry.Lines) < 2 {
        validator.AddError("lines", "At least two journal lines are required")
    } else if len(entry.Lines) > 50 {
        validator.AddError("lines", "Too many journal lines (maximum 50)")
    }

    var totalDebits, totalCredits float64
    for i, line := range entry.Lines {
        fieldPrefix := fmt.Sprintf("lines[%d]", i)
        
        if line.AccountID == 0 {
            validator.AddError(fieldPrefix+".account_id", "Account ID is required")
        }
        
        validator.MinLength(fieldPrefix+".description", line.Description, 3)
        validator.MaxLength(fieldPrefix+".description", line.Description, 200)
        
        if line.DebitAmount < 0 || line.CreditAmount < 0 {
            validator.AddError(fieldPrefix+".amounts", "Amounts cannot be negative")
        }
        if line.DebitAmount > 0 && line.CreditAmount > 0 {
            validator.AddError(fieldPrefix+".amounts", "Line cannot have both debit and credit amounts")
        }
        if line.DebitAmount == 0 && line.CreditAmount == 0 {
            validator.AddError(fieldPrefix+".amounts", "Line must have either debit or credit amount")
        }
        
        // Validate Indonesian Rupiah (no decimals)
        if line.DebitAmount != float64(int64(line.DebitAmount)) {
            validator.AddError(fieldPrefix+".debit_amount", "Indonesian Rupiah amounts should not have decimal places")
        }
        if line.CreditAmount != float64(int64(line.CreditAmount)) {
            validator.AddError(fieldPrefix+".credit_amount", "Indonesian Rupiah amounts should not have decimal places")
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
        // Check for duplicate entry number
        var exists bool
        err := tx.QueryRow(
            "SELECT EXISTS(SELECT 1 FROM journal_entries WHERE company_id = $1 AND entry_number = $2)",
            entry.CompanyID, entry.EntryNumber).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "DUPLICATE_ENTRY", "Entry number already exists")
            return nil
        }

        // Verify all accounts exist and belong to company
        accountIDs := make([]int, len(entry.Lines))
        for i, line := range entry.Lines {
            accountIDs[i] = line.AccountID
        }
        
        if len(accountIDs) > 0 {
            placeholders := make([]string, len(accountIDs))
            args := []interface{}{entry.CompanyID}
            for i, id := range accountIDs {
                placeholders[i] = "$" + strconv.Itoa(i+2)
                args = append(args, id)
            }
            
            verifyQuery := fmt.Sprintf(`SELECT COUNT(*) FROM chart_of_accounts 
                                       WHERE company_id = $1 AND id IN (%s) AND is_active = true`, 
                                       strings.Join(placeholders, ","))
            
            var validAccountCount int
            err := tx.QueryRow(verifyQuery, args...).Scan(&validAccountCount)
            if err != nil {
                return err
            }
            
            if validAccountCount != len(accountIDs) {
                s.RespondWithError(w, http.StatusBadRequest, "INVALID_ACCOUNTS", 
                                 "One or more accounts are invalid or inactive")
                return nil
            }
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
            s.HandleDBError(w, err, "Error creating journal entry")
            return nil
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
                s.HandleDBError(w, err, "Error creating journal entry lines")
                return nil
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
        // Get transaction details and verify ownership
        var entry JournalEntry
        entryQuery := `SELECT id, company_id, entry_number, entry_date, description, total_amount, status, created_by
                       FROM journal_entries WHERE id = $1 AND company_id = $2`
        
        err := tx.QueryRow(entryQuery, id, companyID).Scan(
            &entry.ID, &entry.CompanyID, &entry.EntryNumber, &entry.EntryDate,
            &entry.Description, &entry.TotalAmount, &entry.Status, &entry.CreatedBy)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching transaction")
            return nil
        }
        
        if entry.Status != "draft" {
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_STATUS", 
                             fmt.Sprintf("Cannot post transaction with status: %s", entry.Status))
            return nil
        }
        
        // Get transaction lines
        linesQuery := `SELECT id, journal_entry_id, account_id, description, debit_amount, credit_amount
                       FROM journal_entry_lines WHERE journal_entry_id = $1 ORDER BY id`
        
        rows, err := tx.Query(linesQuery, id)
        if err != nil {
            return err
        }
        defer rows.Close()
        
        var lines []JournalEntryLine
        for rows.Next() {
            var line JournalEntryLine
            err := rows.Scan(&line.ID, &line.JournalEntryID, &line.AccountID,
                            &line.Description, &line.DebitAmount, &line.CreditAmount)
            if err != nil {
                continue
            }
            lines = append(lines, line)
        }
        
        if len(lines) == 0 {
            s.RespondWithError(w, http.StatusBadRequest, "NO_LINES", "Transaction has no lines")
            return nil
        }
        
        // Verify transaction is balanced
        var totalDebits, totalCredits float64
        for _, line := range lines {
            totalDebits += line.DebitAmount
            totalCredits += line.CreditAmount
        }
        
        if abs(totalDebits-totalCredits) > 0.01 {
            s.RespondWithError(w, http.StatusBadRequest, "UNBALANCED", "Transaction is not balanced")
            return nil
        }
        
        // Post to general ledger
        for _, line := range lines {
            ledgerEntry := map[string]interface{}{
                "account_id":       line.AccountID,
                "transaction_date": entry.EntryDate.Format("2006-01-02"),
                "description":      fmt.Sprintf("%s - %s", entry.Description, line.Description),
                "debit_amount":     line.DebitAmount,
                "credit_amount":    line.CreditAmount,
                "reference_id":     entry.EntryNumber,
            }
            
            if err := s.postToLedger(r.Context(), r.Header.Get("Authorization"), ledgerEntry); err != nil {
                s.RespondWithError(w, http.StatusInternalServerError, "POSTING_ERROR", 
                                  fmt.Sprintf("Error posting to ledger: %v", err))
                return nil
            }
        }
        
        // Update transaction status
        now := time.Now()
        updateQuery := `UPDATE journal_entries 
                        SET status = 'posted', posted_by = $1, posted_at = $2, updated_at = CURRENT_TIMESTAMP 
                        WHERE id = $3`
        
        _, err = tx.Exec(updateQuery, userID, now, id)
        if err != nil {
            s.HandleDBError(w, err, "Error updating transaction status")
            return nil
        }
        
        // Return updated transaction
        entry.Status = "posted"
        entry.PostedBy = &userID
        entry.PostedAt = &now
        entry.UpdatedAt = now
        entry.Lines = lines
        
        s.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
            "status":    "posted",
            "posted_at": now,
            "entry":     entry,
        })
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "POST_ERROR", "Transaction posting failed")
    }
}

func (s *TransactionService) reverseTransactionHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
        return
    }
    
    var request struct {
        Reason string `json:"reason"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    if request.Reason == "" {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_REASON", "Reversal reason is required")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)
    userID := s.GetUserIDFromRequest(r)

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Get original transaction
        var entry JournalEntry
        entryQuery := `SELECT id, company_id, entry_number, entry_date, description, total_amount, status
                       FROM journal_entries WHERE id = $1 AND company_id = $2`
        
        err := tx.QueryRow(entryQuery, id, companyID).Scan(
            &entry.ID, &entry.CompanyID, &entry.EntryNumber, &entry.EntryDate,
            &entry.Description, &entry.TotalAmount, &entry.Status)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
            return nil
        }
        if err != nil {
            return err
        }
        
        if entry.Status != "posted" {
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_STATUS", "Only posted transactions can be reversed")
            return nil
        }
        
        // Get original lines
        linesQuery := `SELECT account_id, description, debit_amount, credit_amount
                       FROM journal_entry_lines WHERE journal_entry_id = $1`
        
        rows, err := tx.Query(linesQuery, id)
        if err != nil {
            return err
        }
        defer rows.Close()
        
        var originalLines []JournalEntryLine
        for rows.Next() {
            var line JournalEntryLine
            err := rows.Scan(&line.AccountID, &line.Description, &line.DebitAmount, &line.CreditAmount)
            if err != nil {
                continue
            }
            originalLines = append(originalLines, line)
        }
        
        // Create reversal entry
        reversalNumber := entry.EntryNumber + "-REV"
        reversalDescription := fmt.Sprintf("REVERSAL: %s - %s", entry.Description, request.Reason)
        
        reversalQuery := `INSERT INTO journal_entries (company_id, entry_number, entry_date, description, 
                                                      total_amount, status, created_by) 
                         VALUES ($1, $2, CURRENT_DATE, $3, $4, 'posted', $5) 
                         RETURNING id, created_at, updated_at`
        
        var reversalID int
        var createdAt, updatedAt time.Time
        err = tx.QueryRow(reversalQuery, entry.CompanyID, reversalNumber, reversalDescription,
                         entry.TotalAmount, userID).Scan(&reversalID, &createdAt, &updatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error creating reversal entry")
            return nil
        }
        
        // Create reversed lines (swap debits and credits)
        for _, originalLine := range originalLines {
            lineQuery := `INSERT INTO journal_entry_lines (journal_entry_id, account_id, description, 
                                                           debit_amount, credit_amount) 
                          VALUES ($1, $2, $3, $4, $5)`
            
            _, err = tx.Exec(lineQuery, reversalID, originalLine.AccountID,
                           "Reversal: "+originalLine.Description, originalLine.CreditAmount, originalLine.DebitAmount)
            if err != nil {
                return err
            }
            
            // Post reversed entries to ledger
            ledgerEntry := map[string]interface{}{
                "account_id":       originalLine.AccountID,
                "transaction_date": time.Now().Format("2006-01-02"),
                "description":      reversalDescription + " - Reversal: " + originalLine.Description,
                "debit_amount":     originalLine.CreditAmount,
                "credit_amount":    originalLine.DebitAmount,
                "reference_id":     reversalNumber,
            }
            
            if err := s.postToLedger(r.Context(), r.Header.Get("Authorization"), ledgerEntry); err != nil {
                s.RespondWithError(w, http.StatusInternalServerError, "LEDGER_ERROR", 
                                  fmt.Sprintf("Error posting reversal to ledger: %v", err))
                return nil
            }
        }
        
        // Update original transaction status
        _, err = tx.Exec("UPDATE journal_entries SET status = 'reversed', updated_at = CURRENT_TIMESTAMP WHERE id = $1", id)
        if err != nil {
            return err
        }
        
        response := map[string]interface{}{
            "status":            "reversed",
            "reversal_entry_id": reversalID,
            "reversal_number":   reversalNumber,
            "reversed_at":       time.Now(),
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REVERSAL_ERROR", "Transaction reversal failed")
    }
}

func (s *TransactionService) postToLedger(ctx context.Context, authHeader string, ledgerEntry map[string]interface{}) error {
    jsonData, _ := json.Marshal(ledgerEntry)
    
    req, err := http.NewRequestWithContext(ctx, "POST", s.accountServiceURL+"/ledger", 
                                          bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", authHeader)
    
    resp, err := s.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("ledger posting failed with status %d", resp.StatusCode)
    }
    
    return nil
}

func (s *TransactionService) getTransactionHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    err = s.ExecuteWithTimeout(15*time.Second, func(ctx context.Context) error {
        var entry JournalEntry
        query := `SELECT id, company_id, entry_number, entry_date, description, total_amount, 
                         status, created_by, posted_by, posted_at, created_at, updated_at
                  FROM journal_entries WHERE id = $1 AND company_id = $2`
        
        var postedBy sql.NullInt64
        var postedAt sql.NullTime
        
        err := s.DB.QueryRowContext(ctx, query, id, companyID).Scan(
            &entry.ID, &entry.CompanyID, &entry.EntryNumber, &entry.EntryDate,
            &entry.Description, &entry.TotalAmount, &entry.Status, &entry.CreatedBy,
            &postedBy, &postedAt, &entry.CreatedAt, &entry.UpdatedAt)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Transaction not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching transaction")
            return nil
        }
        
        if postedBy.Valid {
            pb := int(postedBy.Int64)
            entry.PostedBy = &pb
        }
        if postedAt.Valid {
            entry.PostedAt = &postedAt.Time
        }
        
        // Get transaction lines with account information
        linesQuery := `SELECT jel.id, jel.journal_entry_id, jel.account_id, jel.description, 
                              jel.debit_amount, jel.credit_amount, jel.created_at,
                              a.account_code, a.account_name, a.account_type
                       FROM journal_entry_lines jel
                       JOIN chart_of_accounts a ON jel.account_id = a.id
                       WHERE jel.journal_entry_id = $1 ORDER BY jel.id`
        
        rows, err := s.DB.QueryContext(ctx, linesQuery, id)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching transaction lines")
            return nil
        }
        defer rows.Close()
        
        for rows.Next() {
            var line JournalEntryLine
            var account AccountInfo
            
            err := rows.Scan(&line.ID, &line.JournalEntryID, &line.AccountID,
                            &line.Description, &line.DebitAmount, &line.CreditAmount, &line.CreatedAt,
                            &account.AccountCode, &account.AccountName, &account.AccountType)
            if err != nil {
                continue
            }
            
            account.ID = line.AccountID
            line.Account = &account
            entry.Lines = append(entry.Lines, line)
        }
        
        s.RespondWithJSON(w, http.StatusOK, entry)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving transaction")
    }
}

func (s *TransactionService) updateTransactionHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid transaction ID")
        return
    }
    
    var updateData struct {
        Description string `json:"description"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    validator.Required("description", updateData.Description)
    validator.MinLength("description", updateData.Description, 5)
    validator.MaxLength("description", updateData.Description, 500)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check if transaction exists and is in draft status
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
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_STATUS", "Only draft transactions can be updated")
            return nil
        }
        
        // Update transaction
        updateQuery := `UPDATE journal_entries 
                        SET description = $1, updated_at = CURRENT_TIMESTAMP 
                        WHERE id = $2 AND company_id = $3
                        RETURNING updated_at`
        
        var updatedAt time.Time
        err = tx.QueryRow(updateQuery, updateData.Description, id, companyID).Scan(&updatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error updating transaction")
            return nil
        }
        
        response := map[string]interface{}{
            "id":          id,
            "description": updateData.Description,
            "updated_at":  updatedAt,
            "status":      "updated",
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "UPDATE_ERROR", "Transaction update failed")
    }
}

func abs(x float64) float64 {
    if x < 0 {
        return -x
    }
    return x
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}