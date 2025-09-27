// vendor-service/main.go
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

type VendorService struct {
    *service.BaseService
}

type Vendor struct {
    ID           int       `json:"id"`
    CompanyID    int       `json:"company_id"`
    VendorCode   string    `json:"vendor_code"`
    Name         string    `json:"name"`
    Email        string    `json:"email"`
    Phone        string    `json:"phone"`
    Address      string    `json:"address"`
    TaxID        string    `json:"tax_id"`
    PaymentTerms int       `json:"payment_terms"`
    IsActive     bool      `json:"is_active"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

type PurchaseOrder struct {
    ID           int       `json:"id"`
    CompanyID    int       `json:"company_id"`
    VendorID     int       `json:"vendor_id"`
    PONumber     string    `json:"po_number"`
    OrderDate    time.Time `json:"order_date"`
    ExpectedDate time.Time `json:"expected_date"`
    Subtotal     float64   `json:"subtotal"`
    TaxAmount    float64   `json:"tax_amount"`
    TotalAmount  float64   `json:"total_amount"`
    Status       string    `json:"status"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "vendor_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    vendorService := &VendorService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    api := middleware.APIMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(db, "vendor-service")).Methods("GET")
    r.Handle("/vendors", api(vendorService.getVendorsHandler)).Methods("GET")
    r.Handle("/vendors", api(vendorService.createVendorHandler)).Methods("POST")
    r.Handle("/vendors/{id}", api(vendorService.updateVendorHandler)).Methods("PUT")
    r.Handle("/vendors/{id}", api(vendorService.deleteVendorHandler)).Methods("DELETE")
    r.Handle("/purchase-orders", api(vendorService.getPurchaseOrdersHandler)).Methods("GET")
    r.Handle("/purchase-orders", api(vendorService.createPurchaseOrderHandler)).Methods("POST")

    server.SetupServer(r, cfg)
}

func (s *VendorService) getVendorsHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    activeOnly := r.URL.Query().Get("active_only") == "true"
    
    query := `SELECT id, company_id, vendor_code, name, email, phone, address, tax_id, payment_terms, is_active, created_at, updated_at
              FROM vendors WHERE company_id = $1`
    
    args := []interface{}{companyID}
    if activeOnly {
        query += " AND is_active = true"
    }
    query += " ORDER BY name"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching vendors")
        return
    }
    defer rows.Close()
    
    var vendors []Vendor
    for rows.Next() {
        var vendor Vendor
        err := rows.Scan(&vendor.ID, &vendor.CompanyID, &vendor.VendorCode, &vendor.Name,
                        &vendor.Email, &vendor.Phone, &vendor.Address, &vendor.TaxID,
                        &vendor.PaymentTerms, &vendor.IsActive, &vendor.CreatedAt, &vendor.UpdatedAt)
        if err != nil {
            continue
        }
        vendors = append(vendors, vendor)
    }
    
    s.RespondWithJSON(w, http.StatusOK, vendors)
}

func (s *VendorService) createVendorHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    var vendor Vendor
    if err := json.NewDecoder(r.Body).Decode(&vendor); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("vendor_code", vendor.VendorCode)
    validator.Required("name", vendor.Name)
    validator.Email("email", vendor.Email)
    validator.IndonesianTaxID("tax_id", vendor.TaxID)
    
    if vendor.PaymentTerms < 0 || vendor.PaymentTerms > 365 {
        validator.AddError("payment_terms", "Payment terms must be 0-365 days")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    vendor.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    vendor.IsActive = true

    var exists bool
    err := s.DB.QueryRowContext(ctx, 
        "SELECT EXISTS(SELECT 1 FROM vendors WHERE company_id = $1 AND vendor_code = $2)",
        vendor.CompanyID, vendor.VendorCode).Scan(&exists)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error checking duplicate")
        return
    }
    if exists {
        s.RespondWithError(w, http.StatusConflict, "DUPLICATE_CODE", "Vendor code already exists")
        return
    }

    query := `INSERT INTO vendors (company_id, vendor_code, name, email, phone, address, tax_id, payment_terms, is_active) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
              RETURNING id, created_at, updated_at`
    
    err = s.DB.QueryRowContext(ctx, query, 
        vendor.CompanyID, vendor.VendorCode, vendor.Name,
        vendor.Email, vendor.Phone, vendor.Address, 
        vendor.TaxID, vendor.PaymentTerms, vendor.IsActive).Scan(&vendor.ID, &vendor.CreatedAt, &vendor.UpdatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating vendor")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, vendor)
}

func (s *VendorService) updateVendorHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid vendor ID")
        return
    }
    
    var vendor Vendor
    if err := json.NewDecoder(r.Body).Decode(&vendor); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    validator.Required("name", vendor.Name)
    validator.Email("email", vendor.Email)
    validator.IndonesianTaxID("tax_id", vendor.TaxID)
    
    if vendor.PaymentTerms < 0 || vendor.PaymentTerms > 365 {
        validator.AddError("payment_terms", "Payment terms must be 0-365 days")
    }
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `UPDATE vendors 
              SET name = $1, email = $2, phone = $3, address = $4, tax_id = $5, 
                  payment_terms = $6, is_active = $7, updated_at = CURRENT_TIMESTAMP 
              WHERE id = $8 AND company_id = $9 
              RETURNING updated_at`
    
    err = s.DB.QueryRowContext(ctx, query, vendor.Name, vendor.Email, vendor.Phone,
                              vendor.Address, vendor.TaxID, vendor.PaymentTerms, vendor.IsActive,
                              id, companyID).Scan(&vendor.UpdatedAt)
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Vendor not found")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Error updating vendor")
        return
    }
    
    vendor.ID = id
    vendor.CompanyID = companyID
    s.RespondWithJSON(w, http.StatusOK, vendor)
}

func (s *VendorService) deleteVendorHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid vendor ID")
        return
    }
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `UPDATE vendors SET is_active = false, updated_at = CURRENT_TIMESTAMP 
              WHERE id = $1 AND company_id = $2`
    
    result, err := s.DB.ExecContext(ctx, query, id, companyID)
    if err != nil {
        s.HandleDBError(w, err, "Error deleting vendor")
        return
    }
    
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Vendor not found")
        return
    }
    
    s.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *VendorService) getPurchaseOrdersHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `SELECT id, company_id, vendor_id, po_number, order_date, expected_date,
                     subtotal, tax_amount, total_amount, status, created_at, updated_at
              FROM purchase_orders WHERE company_id = $1 ORDER BY created_at DESC`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching purchase orders")
        return
    }
    defer rows.Close()
    
    var orders []PurchaseOrder
    for rows.Next() {
        var order PurchaseOrder
        err := rows.Scan(&order.ID, &order.CompanyID, &order.VendorID, &order.PONumber,
                        &order.OrderDate, &order.ExpectedDate, &order.Subtotal, &order.TaxAmount,
                        &order.TotalAmount, &order.Status, &order.CreatedAt, &order.UpdatedAt)
        if err != nil {
            continue
        }
        orders = append(orders, order)
    }
    
    s.RespondWithJSON(w, http.StatusOK, orders)
}

func (s *VendorService) createPurchaseOrderHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    var order PurchaseOrder
    if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("po_number", order.PONumber)
    if order.VendorID == 0 {
        validator.AddError("vendor_id", "Vendor ID is required")
    }
    validator.PositiveNumber("subtotal", order.Subtotal)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    order.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    order.Status = "draft"
    order.TaxAmount = order.Subtotal * 0.11 // Indonesian PPN
    order.TotalAmount = order.Subtotal + order.TaxAmount

    if order.OrderDate.IsZero() {
        order.OrderDate = time.Now()
    }

    query := `INSERT INTO purchase_orders (company_id, vendor_id, po_number, order_date, expected_date,
                                          subtotal, tax_amount, total_amount, status) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
              RETURNING id, created_at, updated_at`
    
    err := s.DB.QueryRowContext(ctx, query, 
        order.CompanyID, order.VendorID, order.PONumber, order.OrderDate, order.ExpectedDate,
        order.Subtotal, order.TaxAmount, order.TotalAmount, order.Status).Scan(
        &order.ID, &order.CreatedAt, &order.UpdatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating purchase order")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, order)
}