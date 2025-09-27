// company-service/main.go - NEW SERVICE
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

type CompanyService struct {
    *service.BaseService
}

type Company struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    TaxID     string    `json:"tax_id"`
    Address   string    `json:"address"`
    Phone     string    `json:"phone"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func main() {
    cfg := config.ValidateAndLoad()
    cfg.Database.Name = "company_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    companyService := &CompanyService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(db, "company-service")).Methods("GET")
    
    authMiddleware := middleware.APIMiddleware(cfg.JWT.Secret)
    r.Handle("/companies", authMiddleware(companyService.getCompaniesHandler)).Methods("GET")
    r.Handle("/companies", authMiddleware(companyService.createCompanyHandler)).Methods("POST")
    r.Handle("/companies/{id}", authMiddleware(companyService.updateCompanyHandler)).Methods("PUT")

    server.SetupServer(r, cfg)
}

func (s *CompanyService) getCompaniesHandler(w http.ResponseWriter, r *http.Request) {
    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        query := `SELECT id, name, tax_id, address, phone, email, created_at, updated_at
                  FROM companies ORDER BY name`
        
        rows, err := s.DB.QueryContext(ctx, query)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching companies")
            return nil
        }
        defer rows.Close()
        
        var companies []Company
        for rows.Next() {
            var company Company
            err := rows.Scan(&company.ID, &company.Name, &company.TaxID, &company.Address,
                            &company.Phone, &company.Email, &company.CreatedAt, &company.UpdatedAt)
            if err != nil {
                continue
            }
            companies = append(companies, company)
        }
        
        s.RespondWithJSON(w, http.StatusOK, companies)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving companies")
    }
}

func (s *CompanyService) createCompanyHandler(w http.ResponseWriter, r *http.Request) {
    var company Company
    if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("name", company.Name)
    validator.MinLength("name", company.Name, 2)
    validator.MaxLength("name", company.Name, 255)
    validator.Required("tax_id", company.TaxID)
    validator.IndonesianTaxID("tax_id", company.TaxID)
    validator.Email("email", company.Email)
    validator.IndonesianPhone("phone", company.Phone)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check if tax ID already exists
        var exists bool
        err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM companies WHERE tax_id = $1)", company.TaxID).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "TAX_ID_EXISTS", "Company with this Tax ID already exists")
            return nil
        }

        query := `INSERT INTO companies (name, tax_id, address, phone, email) 
                  VALUES ($1, $2, $3, $4, $5) 
                  RETURNING id, created_at, updated_at`
        
        err = tx.QueryRow(query, company.Name, company.TaxID, company.Address,
                         company.Phone, company.Email).Scan(&company.ID, &company.CreatedAt, &company.UpdatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error creating company")
            return nil
        }

        s.RespondWithJSON(w, http.StatusCreated, company)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "CREATE_ERROR", "Company creation failed")
    }
}

func (s *CompanyService) updateCompanyHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid company ID")
        return
    }
    
    var company Company
    if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("name", company.Name)
    validator.Email("email", company.Email)
    validator.IndonesianPhone("phone", company.Phone)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err = s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        query := `UPDATE companies 
                  SET name = $1, address = $2, phone = $3, email = $4, updated_at = CURRENT_TIMESTAMP
                  WHERE id = $5 
                  RETURNING updated_at`
        
        err = tx.QueryRow(query, company.Name, company.Address, company.Phone, company.Email, id).Scan(&company.UpdatedAt)
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Company not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error updating company")
            return nil
        }
        
        company.ID = id
        s.RespondWithJSON(w, http.StatusOK, company)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "UPDATE_ERROR", "Company update failed")
    }
}