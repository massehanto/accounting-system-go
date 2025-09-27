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

type TaxService struct {
    *service.BaseService
}

type TaxRate struct {
    ID        int       `json:"id"`
    CompanyID int       `json:"company_id"`
    TaxName   string    `json:"tax_name"`
    TaxRate   float64   `json:"tax_rate"`
    IsActive  bool      `json:"is_active"`
    CreatedAt time.Time `json:"created_at"`
}

type TaxCalculation struct {
    BaseAmount float64 `json:"base_amount"`
    TaxRate    float64 `json:"tax_rate"`
    TaxAmount  float64 `json:"tax_amount"`
    Total      float64 `json:"total"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "tax_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    taxService := &TaxService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    api := middleware.APIMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(db, "tax-service")).Methods("GET")
    r.Handle("/tax-rates", api(taxService.getTaxRatesHandler)).Methods("GET")
    r.Handle("/tax-rates", api(taxService.createTaxRateHandler)).Methods("POST")
    r.Handle("/calculate-tax", api(taxService.calculateTaxHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *TaxService) getTaxRatesHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `SELECT id, company_id, tax_name, tax_rate, is_active, created_at
              FROM tax_rates WHERE company_id = $1 ORDER BY tax_name`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching tax rates")
        return
    }
    defer rows.Close()
    
    var taxRates []TaxRate
    for rows.Next() {
        var taxRate TaxRate
        err := rows.Scan(&taxRate.ID, &taxRate.CompanyID, &taxRate.TaxName, &taxRate.TaxRate,
                        &taxRate.IsActive, &taxRate.CreatedAt)
        if err != nil {
            continue
        }
        taxRates = append(taxRates, taxRate)
    }
    
    s.RespondWithJSON(w, http.StatusOK, taxRates)
}

func (s *TaxService) createTaxRateHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    var taxRate TaxRate
    if err := json.NewDecoder(r.Body).Decode(&taxRate); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("tax_name", taxRate.TaxName)
    
    if taxRate.TaxRate < 0 || taxRate.TaxRate > 100 {
        validator.AddError("tax_rate", "Tax rate must be between 0 and 100")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    taxRate.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    taxRate.IsActive = true

    query := `INSERT INTO tax_rates (company_id, tax_name, tax_rate, is_active) 
              VALUES ($1, $2, $3, $4) 
              RETURNING id, created_at`
    
    err := s.DB.QueryRowContext(ctx, query, taxRate.CompanyID, taxRate.TaxName, 
                               taxRate.TaxRate, taxRate.IsActive).Scan(&taxRate.ID, &taxRate.CreatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating tax rate")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, taxRate)
}

func (s *TaxService) calculateTaxHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    var req struct {
        Amount    float64 `json:"amount"`
        TaxRateID int     `json:"tax_rate_id"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    if req.Amount <= 0 {
        validator.AddError("amount", "Amount must be positive")
    }
    if req.TaxRateID == 0 {
        validator.AddError("tax_rate_id", "Tax rate ID required")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    var taxRate float64
    err := s.DB.QueryRowContext(ctx, 
        "SELECT tax_rate FROM tax_rates WHERE id = $1 AND company_id = $2 AND is_active = true", 
        req.TaxRateID, companyID).Scan(&taxRate)
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "TAX_RATE_NOT_FOUND", "Tax rate not found")
        return
    }
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Database error")
        return
    }

    taxAmount := req.Amount * (taxRate / 100)
    result := TaxCalculation{
        BaseAmount: req.Amount,
        TaxRate:    taxRate,
        TaxAmount:  taxAmount,
        Total:      req.Amount + taxAmount,
    }

    s.RespondWithJSON(w, http.StatusOK, result)
}