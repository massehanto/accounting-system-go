// report-service/main.go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "strconv"
    "time"
    
    "github.com/gorilla/mux"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
    "github.com/massehanto/accounting-system-go/shared/service"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type ReportService struct {
    *service.BaseService
    serviceURLs map[string]string
    client      *http.Client
}

type ReportRequest struct {
    ReportType string `json:"report_type"`
    StartDate  string `json:"start_date"`
    EndDate    string `json:"end_date"`
    Format     string `json:"format"`
}

type FinancialReport struct {
    ReportType  string                 `json:"report_type"`
    CompanyID   int                    `json:"company_id"`
    Period      string                 `json:"period"`
    Data        map[string]interface{} `json:"data"`
    GeneratedAt time.Time              `json:"generated_at"`
    Totals      map[string]float64     `json:"totals"`
}

type Account struct {
    ID          int     `json:"id"`
    AccountCode string  `json:"account_code"`
    AccountName string  `json:"account_name"`
    AccountType string  `json:"account_type"`
    Balance     float64 `json:"balance"`
}

func main() {
    cfg := config.Load()
    
    reportService := &ReportService{
        BaseService: &service.BaseService{DB: nil},
        serviceURLs: map[string]string{
            "account":     getEnv("ACCOUNT_SERVICE_URL", "http://localhost:8002"),
            "transaction": getEnv("TRANSACTION_SERVICE_URL", "http://localhost:8003"),
            "invoice":     getEnv("INVOICE_SERVICE_URL", "http://localhost:8004"),
        },
        client: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
    
    r := mux.NewRouter()
    api := middleware.APIMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(nil, "report-service")).Methods("GET")
    r.Handle("/reports/generate", api(reportService.generateReportHandler)).Methods("POST")
    r.Handle("/reports/balance-sheet", api(reportService.balanceSheetHandler)).Methods("GET")
    r.Handle("/reports/income-statement", api(reportService.incomeStatementHandler)).Methods("GET")
    r.Handle("/reports/trial-balance", api(reportService.trialBalanceHandler)).Methods("GET")

    server.SetupServer(r, cfg)
}

func (s *ReportService) generateReportHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
    defer cancel()
    
    var req ReportRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("report_type", req.ReportType)
    validator.Required("start_date", req.StartDate)
    validator.Required("end_date", req.EndDate)
    
    validTypes := []string{"balance_sheet", "income_statement", "trial_balance", "cash_flow"}
    if !contains(validTypes, req.ReportType) {
        validator.AddError("report_type", "Invalid report type")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    authHeader := r.Header.Get("Authorization")

    var report *FinancialReport
    var err error

    switch req.ReportType {
    case "balance_sheet":
        report, err = s.generateBalanceSheet(ctx, companyID, authHeader, req.StartDate, req.EndDate)
    case "income_statement":
        report, err = s.generateIncomeStatement(ctx, companyID, authHeader, req.StartDate, req.EndDate)
    case "trial_balance":
        report, err = s.generateTrialBalance(ctx, companyID, authHeader, req.StartDate, req.EndDate)
    default:
        s.RespondWithError(w, http.StatusBadRequest, "UNSUPPORTED_REPORT", "Report type not implemented")
        return
    }

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REPORT_ERROR", err.Error())
        return
    }

    s.RespondWithJSON(w, http.StatusOK, report)
}

func (s *ReportService) fetchAccountData(ctx context.Context, companyID int, authHeader string) ([]Account, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", 
        fmt.Sprintf("%s/accounts?company_id=%d", s.serviceURLs["account"], companyID), nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", authHeader)
    req.Header.Set("Company-ID", strconv.Itoa(companyID))
    
    resp, err := s.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("account service error: %d", resp.StatusCode)
    }
    
    var response struct {
        Data []Account `json:"data"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, err
    }
    
    return response.Data, nil
}

func (s *ReportService) generateBalanceSheet(ctx context.Context, companyID int, authHeader, startDate, endDate string) (*FinancialReport, error) {
    accounts, err := s.fetchAccountData(ctx, companyID, authHeader)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch account data: %v", err)
    }
    
    assets := []map[string]interface{}{}
    liabilities := []map[string]interface{}{}
    equity := []map[string]interface{}{}
    
    var totalAssets, totalLiabilities, totalEquity float64
    
    for _, account := range accounts {
        accountData := map[string]interface{}{
            "code":   account.AccountCode,
            "name":   account.AccountName,
            "amount": account.Balance,
        }
        
        switch account.AccountType {
        case "Asset":
            assets = append(assets, accountData)
            totalAssets += account.Balance
        case "Liability":
            liabilities = append(liabilities, accountData)
            totalLiabilities += account.Balance
        case "Equity":
            equity = append(equity, accountData)
            totalEquity += account.Balance
        }
    }
    
    data := map[string]interface{}{
        "assets":      assets,
        "liabilities": liabilities,
        "equity":      equity,
    }

    return &FinancialReport{
        ReportType:  "balance_sheet",
        CompanyID:   companyID,
        Period:      fmt.Sprintf("%s to %s", startDate, endDate),
        Data:        data,
        GeneratedAt: time.Now(),
        Totals: map[string]float64{
            "total_assets":             totalAssets,
            "total_liabilities":        totalLiabilities,
            "total_equity":            totalEquity,
            "total_liabilities_equity": totalLiabilities + totalEquity,
        },
    }, nil
}

func (s *ReportService) generateIncomeStatement(ctx context.Context, companyID int, authHeader, startDate, endDate string) (*FinancialReport, error) {
    accounts, err := s.fetchAccountData(ctx, companyID, authHeader)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch account data: %v", err)
    }
    
    revenues := []map[string]interface{}{}
    expenses := []map[string]interface{}{}
    
    var totalRevenue, totalExpenses float64
    
    for _, account := range accounts {
        accountData := map[string]interface{}{
            "code":   account.AccountCode,
            "name":   account.AccountName,
            "amount": account.Balance,
        }
        
        switch account.AccountType {
        case "Revenue":
            revenues = append(revenues, accountData)
            totalRevenue += account.Balance
        case "Expense":
            expenses = append(expenses, accountData)
            totalExpenses += account.Balance
        }
    }
    
    netIncome := totalRevenue - totalExpenses
    
    data := map[string]interface{}{
        "revenues": revenues,
        "expenses": expenses,
    }

    return &FinancialReport{
        ReportType:  "income_statement",
        CompanyID:   companyID,
        Period:      fmt.Sprintf("%s to %s", startDate, endDate),
        Data:        data,
        GeneratedAt: time.Now(),
        Totals: map[string]float64{
            "total_revenue":  totalRevenue,
            "total_expenses": totalExpenses,
            "net_income":     netIncome,
        },
    }, nil
}

func (s *ReportService) generateTrialBalance(ctx context.Context, companyID int, authHeader, startDate, endDate string) (*FinancialReport, error) {
    accounts, err := s.fetchAccountData(ctx, companyID, authHeader)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch account data: %v", err)
    }
    
    trialBalance := []map[string]interface{}{}
    var totalDebits, totalCredits float64
    
    for _, account := range accounts {
        var debitAmount, creditAmount float64
        
        if account.Balance > 0 {
            if account.AccountType == "Asset" || account.AccountType == "Expense" {
                debitAmount = account.Balance
                totalDebits += debitAmount
            } else {
                creditAmount = account.Balance
                totalCredits += creditAmount
            }
        }
        
        trialBalance = append(trialBalance, map[string]interface{}{
            "code":          account.AccountCode,
            "name":          account.AccountName,
            "debit_amount":  debitAmount,
            "credit_amount": creditAmount,
        })
    }
    
    data := map[string]interface{}{
        "accounts": trialBalance,
    }

    return &FinancialReport{
        ReportType:  "trial_balance",
        CompanyID:   companyID,
        Period:      fmt.Sprintf("%s to %s", startDate, endDate),
        Data:        data,
        GeneratedAt: time.Now(),
        Totals: map[string]float64{
            "total_debits":  totalDebits,
            "total_credits": totalCredits,
        },
    }, nil
}

func (s *ReportService) balanceSheetHandler(w http.ResponseWriter, r *http.Request) {
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    authHeader := r.Header.Get("Authorization")
    
    startDate := r.URL.Query().Get("start_date")
    endDate := r.URL.Query().Get("end_date")
    
    if startDate == "" || endDate == "" {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_DATES", "Start and end dates required")
        return
    }
    
    report, err := s.generateBalanceSheet(r.Context(), companyID, authHeader, startDate, endDate)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REPORT_ERROR", err.Error())
        return
    }
    
    s.RespondWithJSON(w, http.StatusOK, report)
}

func (s *ReportService) incomeStatementHandler(w http.ResponseWriter, r *http.Request) {
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    authHeader := r.Header.Get("Authorization")
    
    startDate := r.URL.Query().Get("start_date")
    endDate := r.URL.Query().Get("end_date")
    
    if startDate == "" || endDate == "" {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_DATES", "Start and end dates required")
        return
    }
    
    report, err := s.generateIncomeStatement(r.Context(), companyID, authHeader, startDate, endDate)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REPORT_ERROR", err.Error())
        return
    }
    
    s.RespondWithJSON(w, http.StatusOK, report)
}

func (s *ReportService) trialBalanceHandler(w http.ResponseWriter, r *http.Request) {
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    authHeader := r.Header.Get("Authorization")
    
    startDate := r.URL.Query().Get("start_date")
    endDate := r.URL.Query().Get("end_date")
    
    if startDate == "" || endDate == "" {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_DATES", "Start and end dates required")
        return
    }
    
    report, err := s.generateTrialBalance(r.Context(), companyID, authHeader, startDate, endDate)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REPORT_ERROR", err.Error())
        return
    }
    
    s.RespondWithJSON(w, http.StatusOK, report)
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}