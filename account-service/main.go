// account-service/main.go
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
    Account         *Account  `json:"account,omitempty"`
}

func main() {
    cfg := config.ValidateAndLoad()
    cfg.Database.Name = "account_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    accountService := &AccountService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    
    // Health check
    r.Handle("/health", middleware.HealthCheck(db, "account-service")).Methods("GET")
    
    // Account endpoints with enhanced middleware
    authMiddleware := middleware.APIMiddleware(cfg.JWT.Secret)
    r.Handle("/accounts", authMiddleware(accountService.getAccountsHandler)).Methods("GET")
    r.Handle("/accounts", authMiddleware(accountService.createAccountHandler)).Methods("POST")
    r.Handle("/accounts/{id}", authMiddleware(accountService.getAccountHandler)).Methods("GET")
    r.Handle("/accounts/{id}", authMiddleware(accountService.updateAccountHandler)).Methods("PUT")
    r.Handle("/accounts/{id}/balance", authMiddleware(accountService.getAccountBalanceHandler)).Methods("GET")
    
    // Ledger endpoints
    r.Handle("/ledger", authMiddleware(accountService.getLedgerHandler)).Methods("GET")
    r.Handle("/ledger", authMiddleware(accountService.createLedgerEntryHandler)).Methods("POST")
    r.Handle("/ledger/{id}", authMiddleware(accountService.getLedgerEntryHandler)).Methods("GET")

    server.SetupServer(r, cfg)
}

func (s *AccountService) getAccountsHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID is required")
        return
    }
    
    accountType := r.URL.Query().Get("type")
    activeOnly := r.URL.Query().Get("active_only") == "true"
    includeBalance := r.URL.Query().Get("include_balance") == "true"

    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        query := `SELECT a.id, a.company_id, a.account_code, a.account_name, a.account_type, 
                         a.parent_id, a.is_active, a.created_at, a.updated_at`
        
        if includeBalance {
            query += `, COALESCE(SUM(
                CASE 
                    WHEN a.account_type IN ('Asset', 'Expense') THEN gl.debit_amount - gl.credit_amount
                    ELSE gl.credit_amount - gl.debit_amount
                END
            ), 0) as balance`
        } else {
            query += `, 0 as balance`
        }
        
        query += ` FROM chart_of_accounts a`
        
        if includeBalance {
            query += ` LEFT JOIN general_ledger gl ON a.id = gl.account_id`
        }
        
        query += ` WHERE a.company_id = $1`
        
        args := []interface{}{companyID}
        argCount := 1
        
        if accountType != "" {
            argCount++
            query += " AND a.account_type = $" + strconv.Itoa(argCount)
            args = append(args, accountType)
        }
        
        if activeOnly {
            query += " AND a.is_active = true"
        }
        
        if includeBalance {
            query += " GROUP BY a.id, a.company_id, a.account_code, a.account_name, a.account_type, a.parent_id, a.is_active, a.created_at, a.updated_at"
        }
        
        query += " ORDER BY a.account_code"
        
        rows, err := s.DB.QueryContext(ctx, query, args...)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching accounts")
            return nil
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

        if err = rows.Err(); err != nil {
            s.HandleDBError(w, err, "Error processing accounts")
            return nil
        }

        s.RespondWithJSON(w, http.StatusOK, accounts)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving accounts")
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
    if !s.ValidateCompanyAccess(r, companyID) {
        s.RespondWithError(w, http.StatusForbidden, "ACCESS_DENIED", "Access denied")
        return
    }

    err = s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
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
                  GROUP BY a.id, a.company_id, a.account_code, a.account_name, a.account_type, 
                           a.parent_id, a.is_active, a.created_at, a.updated_at`
        
        err := s.DB.QueryRowContext(ctx, query, id, companyID).Scan(
            &account.ID, &account.CompanyID, &account.AccountCode,
            &account.AccountName, &account.AccountType, &parentID,
            &account.IsActive, &account.CreatedAt, &account.UpdatedAt, &account.Balance)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching account")
            return nil
        }
        
        if parentID.Valid {
            pid := int(parentID.Int64)
            account.ParentID = &pid
        }
        
        s.RespondWithJSON(w, http.StatusOK, account)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving account")
    }
}

func (s *AccountService) createAccountHandler(w http.ResponseWriter, r *http.Request) {
    var account Account
    if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    // Enhanced validation
    validator := validation.New()
    validator.Required("account_code", account.AccountCode)
    validator.AccountCode("account_code", account.AccountCode)
    validator.Required("account_name", account.AccountName)
    validator.MinLength("account_name", account.AccountName, 2)
    validator.MaxLength("account_name", account.AccountName, 255)
    validator.Required("account_type", account.AccountType)
    validator.ValidAccountType("account_type", account.AccountType)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    account.CompanyID = s.GetCompanyIDFromRequest(r)
    account.IsActive = true

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check for duplicate account code
        var exists bool
        err := tx.QueryRow(
            "SELECT EXISTS(SELECT 1 FROM chart_of_accounts WHERE company_id = $1 AND account_code = $2)",
            account.CompanyID, account.AccountCode).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "DUPLICATE_CODE", "Account code already exists")
            return nil
        }

        // Verify parent account if specified
        if account.ParentID != nil {
            var parentExists bool
            var parentType string
            err = tx.QueryRow(
                "SELECT EXISTS(SELECT 1 FROM chart_of_accounts WHERE id = $1 AND company_id = $2 AND is_active = true), account_type",
                *account.ParentID, account.CompanyID).Scan(&parentExists, &parentType)
            if err != nil {
                return err
            }
            if !parentExists {
                s.RespondWithError(w, http.StatusBadRequest, "INVALID_PARENT", "Parent account not found or inactive")
                return nil
            }
            if parentType != account.AccountType {
                s.RespondWithError(w, http.StatusBadRequest, "PARENT_TYPE_MISMATCH", "Parent account must be of the same type")
                return nil
            }
        }

        query := `INSERT INTO chart_of_accounts (company_id, account_code, account_name, account_type, parent_id, is_active) 
                  VALUES ($1, $2, $3, $4, $5, $6) 
                  RETURNING id, created_at, updated_at`
        
        err = tx.QueryRow(query, 
            account.CompanyID, account.AccountCode, account.AccountName, 
            account.AccountType, account.ParentID, account.IsActive).Scan(
            &account.ID, &account.CreatedAt, &account.UpdatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error creating account")
            return nil
        }

        s.RespondWithJSON(w, http.StatusCreated, account)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Account creation failed")
    }
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
    
    // Enhanced validation
    validator := validation.New()
    validator.Required("account_name", account.AccountName)
    validator.MinLength("account_name", account.AccountName, 2)
    validator.MaxLength("account_name", account.AccountName, 255)
    validator.Required("account_type", account.AccountType)
    validator.ValidAccountType("account_type", account.AccountType)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Verify account exists and belongs to company
        var currentType string
        err := tx.QueryRow("SELECT account_type FROM chart_of_accounts WHERE id = $1 AND company_id = $2", 
                          id, companyID).Scan(&currentType)
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
            return nil
        }
        if err != nil {
            return err
        }

        // Check if account has transactions before allowing type change
        if currentType != account.AccountType {
            var transactionCount int
            err = tx.QueryRow("SELECT COUNT(*) FROM general_ledger WHERE account_id = $1", id).Scan(&transactionCount)
            if err != nil {
                return err
            }
            if transactionCount > 0 {
                s.RespondWithError(w, http.StatusBadRequest, "HAS_TRANSACTIONS", 
                                 "Cannot change account type - account has existing transactions")
                return nil
            }
        }

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
            s.HandleDBError(w, err, "Error updating account")
            return nil
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

func (s *AccountService) getAccountBalanceHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid account ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)
    asOf := r.URL.Query().Get("as_of")

    err = s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        query := `SELECT a.account_code, a.account_name, a.account_type,
                         COALESCE(SUM(
                             CASE 
                                 WHEN a.account_type IN ('Asset', 'Expense') THEN gl.debit_amount - gl.credit_amount
                                 ELSE gl.credit_amount - gl.debit_amount
                             END
                         ), 0) as balance
                  FROM chart_of_accounts a
                  LEFT JOIN general_ledger gl ON a.id = gl.account_id`
        
        args := []interface{}{id, companyID}
        
        if asOf != "" {
            query += " AND gl.transaction_date <= $3"
            args = append(args, asOf)
        }
        
        query += ` WHERE a.id = $1 AND a.company_id = $2
                   GROUP BY a.id, a.account_code, a.account_name, a.account_type`
        
        var balance struct {
            AccountCode string  `json:"account_code"`
            AccountName string  `json:"account_name"`
            AccountType string  `json:"account_type"`
            Balance     float64 `json:"balance"`
            AsOfDate    string  `json:"as_of_date,omitempty"`
        }
        
        err := s.DB.QueryRowContext(ctx, query, args...).Scan(
            &balance.AccountCode, &balance.AccountName, &balance.AccountType, &balance.Balance)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Account not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching account balance")
            return nil
        }
        
        if asOf != "" {
            balance.AsOfDate = asOf
        }
        
        s.RespondWithJSON(w, http.StatusOK, balance)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "BALANCE_ERROR", "Error retrieving account balance")
    }
}

func (s *AccountService) getLedgerHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID is required")
        return
    }
    
    // Enhanced query parameters
    accountID := r.URL.Query().Get("account_id")
    startDate := r.URL.Query().Get("start_date")
    endDate := r.URL.Query().Get("end_date")
    limit := r.URL.Query().Get("limit")
    offset := r.URL.Query().Get("offset")
    
    if limit == "" {
        limit = "100"
    }
    if offset == "" {
        offset = "0"
    }

    err := s.ExecuteWithTimeout(15*time.Second, func(ctx context.Context) error {
        query := `SELECT gl.id, gl.company_id, gl.account_id, gl.transaction_date, gl.description, 
                         gl.debit_amount, gl.credit_amount, gl.reference_id, gl.created_at,
                         a.account_code, a.account_name, a.account_type
                  FROM general_ledger gl
                  JOIN chart_of_accounts a ON gl.account_id = a.id
                  WHERE gl.company_id = $1`
        
        args := []interface{}{companyID}
        argCount := 1
        
        if accountID != "" {
            argCount++
            query += " AND gl.account_id = $" + strconv.Itoa(argCount)
            args = append(args, accountID)
        }
        
        if startDate != "" {
            argCount++
            query += " AND gl.transaction_date >= $" + strconv.Itoa(argCount)
            args = append(args, startDate)
        }
        
        if endDate != "" {
            argCount++
            query += " AND gl.transaction_date <= $" + strconv.Itoa(argCount)
            args = append(args, endDate)
        }
        
        query += " ORDER BY gl.transaction_date DESC, gl.created_at DESC"
        
        // Add pagination
        argCount++
        query += " LIMIT $" + strconv.Itoa(argCount)
        args = append(args, limit)
        
        argCount++
        query += " OFFSET $" + strconv.Itoa(argCount)
        args = append(args, offset)
        
        rows, err := s.DB.QueryContext(ctx, query, args...)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching ledger")
            return nil
        }
        defer rows.Close()
        
        var ledger []GeneralLedger
        for rows.Next() {
            var entry GeneralLedger
            var account Account
            
            err := rows.Scan(&entry.ID, &entry.CompanyID, &entry.AccountID, 
                            &entry.TransactionDate, &entry.Description, &entry.DebitAmount, 
                            &entry.CreditAmount, &entry.ReferenceID, &entry.CreatedAt,
                            &account.AccountCode, &account.AccountName, &account.AccountType)
            if err != nil {
                continue
            }
            
            account.ID = entry.AccountID
            entry.Account = &account
            ledger = append(ledger, entry)
        }
        
        if err = rows.Err(); err != nil {
            s.HandleDBError(w, err, "Error processing ledger")
            return nil
        }
        
        // Get total count for pagination
        countQuery := `SELECT COUNT(*) FROM general_ledger gl WHERE gl.company_id = $1`
        countArgs := []interface{}{companyID}
        
        if accountID != "" {
            countQuery += " AND gl.account_id = $2"
            countArgs = append(countArgs, accountID)
        }
        
        var totalCount int
        err = s.DB.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
        if err != nil {
            totalCount = 0 // Don't fail if count query fails
        }
        
        response := map[string]interface{}{
            "data":        ledger,
            "total_count": totalCount,
            "limit":       limit,
            "offset":      offset,
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "LEDGER_ERROR", "Error retrieving ledger")
    }
}

func (s *AccountService) getLedgerEntryHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid ledger entry ID")
        return
    }
    
    companyID := s.GetCompanyIDFromRequest(r)

    err = s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        var entry GeneralLedger
        var account Account
        
        query := `SELECT gl.id, gl.company_id, gl.account_id, gl.transaction_date, gl.description, 
                         gl.debit_amount, gl.credit_amount, gl.reference_id, gl.created_at,
                         a.account_code, a.account_name, a.account_type
                  FROM general_ledger gl
                  JOIN chart_of_accounts a ON gl.account_id = a.id
                  WHERE gl.id = $1 AND gl.company_id = $2`
        
        err := s.DB.QueryRowContext(ctx, query, id, companyID).Scan(
            &entry.ID, &entry.CompanyID, &entry.AccountID,
            &entry.TransactionDate, &entry.Description, &entry.DebitAmount,
            &entry.CreditAmount, &entry.ReferenceID, &entry.CreatedAt,
            &account.AccountCode, &account.AccountName, &account.AccountType)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Ledger entry not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching ledger entry")
            return nil
        }
        
        account.ID = entry.AccountID
        entry.Account = &account
        
        s.RespondWithJSON(w, http.StatusOK, entry)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "LEDGER_ENTRY_ERROR", "Error retrieving ledger entry")
    }
}

func (s *AccountService) createLedgerEntryHandler(w http.ResponseWriter, r *http.Request) {
    var entry GeneralLedger
    if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    // Enhanced validation
    validator := validation.New()
    if entry.AccountID == 0 {
        validator.AddError("account_id", "Account ID is required")
    }
    validator.Required("description", entry.Description)
    validator.MinLength("description", entry.Description, 3)
    validator.MaxLength("description", entry.Description, 500)
    
    if entry.DebitAmount < 0 || entry.CreditAmount < 0 {
        validator.AddError("amounts", "Amounts cannot be negative")
    }
    
    if entry.DebitAmount > 0 && entry.CreditAmount > 0 {
        validator.AddError("amounts", "Entry cannot have both debit and credit amounts")
    }
    
    if entry.DebitAmount == 0 && entry.CreditAmount == 0 {
        validator.AddError("amounts", "Entry must have either debit or credit amount")
    }
    
    // Validate currency amounts for Indonesian Rupiah
    if entry.DebitAmount != float64(int64(entry.DebitAmount)) {
        validator.AddError("debit_amount", "Indonesian Rupiah amounts should not have decimal places")
    }
    if entry.CreditAmount != float64(int64(entry.CreditAmount)) {
        validator.AddError("credit_amount", "Indonesian Rupiah amounts should not have decimal places")
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
        // Verify account exists and belongs to company
        var accountExists bool
        err := tx.QueryRow(
            "SELECT EXISTS(SELECT 1 FROM chart_of_accounts WHERE id = $1 AND company_id = $2 AND is_active = true)",
            entry.AccountID, entry.CompanyID).Scan(&accountExists)
        if err != nil {
            return err
        }
        if !accountExists {
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_ACCOUNT", "Account not found or inactive")
            return nil
        }
        
        query := `INSERT INTO general_ledger (company_id, account_id, transaction_date, description, 
                                              debit_amount, credit_amount, reference_id) 
                  VALUES ($1, $2, $3, $4, $5, $6, $7) 
                  RETURNING id, created_at`
        
        err = tx.QueryRow(query, entry.CompanyID, entry.AccountID, 
                         entry.TransactionDate, entry.Description, entry.DebitAmount, 
                         entry.CreditAmount, entry.ReferenceID).Scan(&entry.ID, &entry.CreatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error creating ledger entry")
            return nil
        }
        
        s.RespondWithJSON(w, http.StatusCreated, entry)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Ledger entry creation failed")
    }
}