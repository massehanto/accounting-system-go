// report-service/main.go - DECOUPLED VERSION
package main

import (
    "context"
    "encoding/json"
    "net/http"
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
}

type ReportRequest struct {
    ReportType string `json:"report_type"`
    StartDate  string `json:"start_date"`
    EndDate    string `json:"end_date"`
}

type FinancialReport struct {
    ReportType  string                 `json:"report_type"`
    CompanyID   int                    `json:"company_id"`
    Period      string                 `json:"period"`
    Data        map[string]interface{} `json:"data"`
    GeneratedAt time.Time              `json:"generated_at"`
    Message     string                 `json:"message"`
}

func main() {
    cfg := config.Load()
    
    reportService := &ReportService{
        BaseService: &service.BaseService{DB: nil},
    }
    
    r := mux.NewRouter()
    authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(nil, "report-service")).Methods("GET")
    r.Handle("/reports/generate", authMiddleware(reportService.generateReportHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *ReportService) generateReportHandler(w http.ResponseWriter, r *http.Request) {
    var req ReportRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("report_type", req.ReportType)
    validator.Required("start_date", req.StartDate)
    validator.Required("end_date", req.EndDate)
    
    validTypes := []string{"balance_sheet", "income_statement", "trial_balance"}
    validator.OneOf("report_type", req.ReportType, validTypes)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    companyID := s.GetCompanyIDFromRequest(r)

    // In a properly decoupled architecture, this would:
    // 1. Query a read-only reporting database
    // 2. Use cached/materialized views
    // 3. Consume events from other services
    
    report := &FinancialReport{
        ReportType:  req.ReportType,
        CompanyID:   companyID,
        Period:      req.StartDate + " to " + req.EndDate,
        GeneratedAt: time.Now(),
        Message:     "This is a sample report. In production, this would contain real financial data from a dedicated reporting database.",
        Data: map[string]interface{}{
            "sample_data": true,
            "explanation": "Reports should be generated from read-only replicas or materialized views, not by calling other services directly",
            "architecture_note": "Consider implementing CQRS pattern with dedicated read models for reporting",
        },
    }

    s.RespondWithJSON(w, http.StatusOK, report)
}