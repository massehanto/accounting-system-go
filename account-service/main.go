// account-service/main.go - SIMPLIFIED VERSION
package main

import (
    "context"
    "database/sql"
    "encoding/json"
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

type AccountService struct {
    *service.BaseService
}

type Account struct {
    ID          int       `json:"id"`
    CompanyID   int       `json:"company_id"`
    AccountCode string    `json:"account_code"`
    AccountName string    `json:"account_name"`
    AccountType string    `json:"account_type"`
    ParentID    *int      `json:"parent_id"`
    IsActive    bool      `json:"is_active"`
    Balance     float64   `json:"balance"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type GeneralLedger struct {
    ID              int       `json:"id"`
    CompanyID       int       `json:"company_id"`
    AccountID       int       `json:"account_id"`
    TransactionDate time.Time `json:"transaction_date"`
    Description     string    `json:"description"`
    DebitAmount     float64   `json:"debit_amount"`
    CreditAmount    float64   `json:"credit_amount"`
    ReferenceID     string    `json:"reference_id"`
    CreatedAt       time.Time `json:"created_at"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "account_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    accountService := &AccountService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(db, "account-service")).Methods("GET")
    
    authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)
    r.Handle("/accounts", authMiddleware(accountService.getAccountsHandler)).Methods("GET")
    r.Handle("/accounts", authMiddleware(accountService.createAccountHandler)).Methods("POST")
    r.Handle("/accounts/{id}", authMiddleware(accountService.getAccountHandler)).Methods("GET")
    r.Handle("/accounts/{id}", authMiddleware(accountService.updateAccountHandler)).Methods("PUT")
    r.Handle("/ledger", authMiddleware(accountService.getLedgerHandler)).Methods("GET")
    r.Handle("/ledger", authMiddleware(accountService.createLedgerEntryHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *AccountService) getAccountsHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID required")
        return
    }
    
    accountType := r.URL.Query().Get("type")
    activeOnly := r.URL.Query().Get("active_only") == "true"

    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    query := `SELECT a.id, a.company_id, a.account_code, a.account_name, a.account_type, 
                     a.parent_id, a.is_active, a.created_at, a.updated_at,
                     COALESCE(SUM(
                         CASE 
                             WHEN a.account_type IN ('Asset', 'Expense') THEN gl.debit_amount - gl.credit_amount
                             ELSE gl.credit_amount - gl.debit_amount
                         END
                     ), 0) as balance
              FROM chart_of_accounts a
              LEFT JOIN general_ledger gl ON a.id = gl.account_id
              WHERE a.company_id = $1`
    
    args := []interface{}{companyID}
    
    if accountType != "" {
        query += " AND a.account_type = $2"
        args = append(args, accountType)
    }
    
    if activeOnly {
        query += " AND a.is_active = true"
    }
    
    query += " GROUP BY a.id ORDER BY a.account_code"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.HandleDBError(w, err, "Error fetching accounts")
        return
    }
    defer rows.Close()

    var accounts []Account
    for rows.Next() {
        var account Account
        var parentID sql.NullInt64
        
        err := rows.Scan(
            &account.ID, &account.CompanyID, &account.AccountCode, 
            &account.AccountName, &account.AccountType, &parentID,
            &account.IsActive, &account.CreatedAt, &account.UpdatedAt, &account.Balance)
        if err != nil {
            continue
        }
        
        if parentID.Valid {
            pid := int(parentID.Int64)
            account.ParentID = &pid
        }
        
        accounts = append(accounts, account)
    }

    s.RespondWithJSON(w, http.StatusOK, accounts)
}

func (s *AccountService) createAccountHandler(w http.ResponseWriter, r *http.Request) {
    var account Account
    if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("account_code", account.AccountCode)
    validator.AccountCode("account_code", account.AccountCode)
    validator.Required("account_name", account.AccountName)
    validator.Required("account_type", account.AccountType)
    
    validTypes := []string{"Asset", "Liability", "Equity", "Revenue", "Expense"}
    validator.OneOf("account_type", account.AccountType, validTypes)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    account.CompanyID = s.GetCompanyIDFromRequest(r)
    account.IsActive = true

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check duplicate code
        var exists bool
        err := tx.QueryRow(
            "SELECT EXISTS(SELECT 1 FROM chart_of_accounts WHERE company_id = $1 AND account_code = $2)",
            account.CompanyID, account.AccountCode).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "DUPLICATE_CODE", "Account code exists")
            return nil
        }

        query := `INSERT INTO chart_of_accounts (company_id, account_code, account_name, account_type, parent_id, is_active) 
                  VALUES ($1, $2, $3, $4, $5, $6) 
                  RETURNING id, created_at, updated_at`
        
        err = tx.QueryRow(query, 
            account.CompanyID, account.AccountCode, account.AccountName, 
            account.AccountType, account.ParentID, account.IsActive).Scan(
            &account.ID, &account.CreatedAt, &account.UpdatedAt)
        if err != nil {
            return err
        }

        s.RespondWithJSON(w, http.StatusCreated, account)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Account creation failed")
    }
}

func (s *AccountService) getAccountHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid account ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    var account Account
    var parentID sql.NullInt64
    
    query := `SELECT a.id, a.company_id, a.account_code, a.account_name, a.account_type, 
                     a.parent_id, a.is_active, a.created_at, a.updated_at,
                     COALESCE(SUM(
                         CASE 
                             WHEN a.account_type IN ('Asset', 'Expense') THEN gl.debit_amount - gl.credit_amount
                             ELSE gl.credit_amount - gl.debit_amount
                         END
                     ), 0) as balance
              FROM chart_of_accounts a
              LEFT JOIN general_ledger gl ON a.id = gl.account_id
              WHERE a.id = $1 AND a.company_id = $2
              GROUP BY a.id`
    
    err = s.DB.QueryRowContext(ctx, query, id, companyID).Scan(
        &account.ID, &account.CompanyID, &account.AccountCode,
        &account.AccountName, &account.AccountType, &parentID,
        &account.IsActive, &account.CreatedAt, &account.UpdatedAt, &account.Balance)
    
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Error fetching account")
        return
    }
    
    if parentID.Valid {
        pid := int(parentID.Int64)
        account.ParentID = &pid
    }
    
    s.RespondWithJSON(w, http.StatusOK, account)
}

func (s *AccountService) updateAccountHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid account ID")
        return
    }
    
    var account Account
    if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    validator.Required("account_name", account.AccountName)
    validator.Required("account_type", account.AccountType)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        query := `UPDATE chart_of_accounts 
                  SET account_name = $1, account_type = $2, parent_id = $3, is_active = $4, updated_at = CURRENT_TIMESTAMP 
                  WHERE id = $5 AND company_id = $6 
                  RETURNING updated_at`
        
        err = tx.QueryRow(query, account.AccountName, account.AccountType, 
                         account.ParentID, account.IsActive, id, companyID).Scan(&account.UpdatedAt)
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
            return nil
        }
        if err != nil {
            return err
        }
        
        account.ID = id
        account.CompanyID = companyID
        s.RespondWithJSON(w, http.StatusOK, account)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "UPDATE_ERROR", "Account update failed")
    }
}

func (s *AccountService) getLedgerHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    accountID := r.URL.Query().Get("account_id")
    
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    query := `SELECT id, company_id, account_id, transaction_date, description, 
                     debit_amount, credit_amount, reference_id, created_at
              FROM general_ledger 
              WHERE company_id = $1`
    
    args := []interface{}{companyID}
    
    if accountID != "" {
        query += " AND account_id = $2"
        args = append(args, accountID)
    }
    
    query += " ORDER BY transaction_date DESC, created_at DESC LIMIT 100"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.HandleDBError(w, err, "Error fetching ledger")
        return
    }
    defer rows.Close()
    
    var ledger []GeneralLedger
    for rows.Next() {
        var entry GeneralLedger
        
        err := rows.Scan(&entry.ID, &entry.CompanyID, &entry.AccountID, 
                        &entry.TransactionDate, &entry.Description, &entry.DebitAmount, 
                        &entry.CreditAmount, &entry.ReferenceID, &entry.CreatedAt)
        if err != nil {
            continue
        }
        
        ledger = append(ledger, entry)
    }
    
    s.RespondWithJSON(w, http.StatusOK, ledger)
}

func (s *AccountService) createLedgerEntryHandler(w http.ResponseWriter, r *http.Request) {
    var entry GeneralLedger
    if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    if entry.AccountID == 0 {
        validator.AddError("account_id", "Account ID required")
    }
    validator.Required("description", entry.Description)
    
    if entry.DebitAmount < 0 || entry.CreditAmount < 0 {
        validator.AddError("amounts", "Amounts cannot be negative")
    }
    
    if entry.DebitAmount > 0 && entry.CreditAmount > 0 {
        validator.AddError("amounts", "Cannot have both debit and credit")
    }
    
    if entry.DebitAmount == 0 && entry.CreditAmount == 0 {
        validator.AddError("amounts", "Must have debit or credit amount")
    }
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    entry.CompanyID = s.GetCompanyIDFromRequest(r)
    
    if entry.TransactionDate.IsZero() {
        entry.TransactionDate = time.Now()
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        query := `INSERT INTO general_ledger (company_id, account_id, transaction_date, description, 
                                              debit_amount, credit_amount, reference_id) 
                  VALUES ($1, $2, $3, $4, $5, $6, $7) 
                  RETURNING id, created_at`
        
        err := tx.QueryRow(query, entry.CompanyID, entry.AccountID, 
                         entry.TransactionDate, entry.Description, entry.DebitAmount, 
                         entry.CreditAmount, entry.ReferenceID).Scan(&entry.ID, &entry.CreatedAt)
        if err != nil {
            return err
        }
        
        s.RespondWithJSON(w, http.StatusCreated, entry)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Ledger entry creation failed")
    }
}