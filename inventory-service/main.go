// inventory-service/main.go
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

type InventoryService struct {
    *service.BaseService
}

type Product struct {
    ID             int       `json:"id"`
    CompanyID      int       `json:"company_id"`
    ProductCode    string    `json:"product_code"`
    ProductName    string    `json:"product_name"`
    Description    string    `json:"description"`
    UnitPrice      float64   `json:"unit_price"`
    CostPrice      float64   `json:"cost_price"`
    QuantityOnHand int       `json:"quantity_on_hand"`
    MinimumStock   int       `json:"minimum_stock"`
    IsActive       bool      `json:"is_active"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type StockMovement struct {
    ID              int       `json:"id"`
    CompanyID       int       `json:"company_id"`
    ProductID       int       `json:"product_id"`
    MovementType    string    `json:"movement_type"`
    Quantity        int       `json:"quantity"`
    UnitCost        float64   `json:"unit_cost"`
    ReferenceNumber string    `json:"reference_number"`
    MovementDate    time.Time `json:"movement_date"`
    Notes           string    `json:"notes"`
    CreatedBy       int       `json:"created_by"`
    CreatedAt       time.Time `json:"created_at"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "inventory_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    inventoryService := &InventoryService{
        BaseService: &service.BaseService{DB: db},
    }
    
    r := mux.NewRouter()
    api := middleware.APIMiddleware(cfg.JWT.Secret)
    
    r.Handle("/health", middleware.HealthCheck(db, "inventory-service")).Methods("GET")
    r.Handle("/products", api(inventoryService.getProductsHandler)).Methods("GET")
    r.Handle("/products", api(inventoryService.createProductHandler)).Methods("POST")
    r.Handle("/products/{id}", api(inventoryService.updateProductHandler)).Methods("PUT")
    r.Handle("/products/{id}", api(inventoryService.deleteProductHandler)).Methods("DELETE")
    r.Handle("/stock-movements", api(inventoryService.getStockMovementsHandler)).Methods("GET")
    r.Handle("/stock-movements", api(inventoryService.createStockMovementHandler)).Methods("POST")
    r.Handle("/low-stock", api(inventoryService.getLowStockHandler)).Methods("GET")

    server.SetupServer(r, cfg)
}

func (s *InventoryService) getProductsHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    activeOnly := r.URL.Query().Get("active_only") == "true"
    
    query := `SELECT id, company_id, product_code, product_name, description, 
                     unit_price, cost_price, quantity_on_hand, minimum_stock, 
                     is_active, created_at, updated_at
              FROM products WHERE company_id = $1`
    
    args := []interface{}{companyID}
    if activeOnly {
        query += " AND is_active = true"
    }
    query += " ORDER BY product_code"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching products")
        return
    }
    defer rows.Close()
    
    var products []Product
    for rows.Next() {
        var product Product
        err := rows.Scan(&product.ID, &product.CompanyID, &product.ProductCode, 
                        &product.ProductName, &product.Description, &product.UnitPrice, 
                        &product.CostPrice, &product.QuantityOnHand, &product.MinimumStock,
                        &product.IsActive, &product.CreatedAt, &product.UpdatedAt)
        if err != nil {
            continue
        }
        products = append(products, product)
    }
    
    s.RespondWithJSON(w, http.StatusOK, products)
}

func (s *InventoryService) createProductHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    var product Product
    if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("product_code", product.ProductCode)
    validator.Required("product_name", product.ProductName)
    
    if product.UnitPrice < 0 {
        validator.AddError("unit_price", "Unit price cannot be negative")
    }
    if product.CostPrice < 0 {
        validator.AddError("cost_price", "Cost price cannot be negative")
    }
    if product.MinimumStock < 0 {
        validator.AddError("minimum_stock", "Minimum stock cannot be negative")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    product.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    product.IsActive = true

    // Check for duplicate product code
    var exists bool
    err := s.DB.QueryRowContext(ctx, 
        "SELECT EXISTS(SELECT 1 FROM products WHERE company_id = $1 AND product_code = $2)",
        product.CompanyID, product.ProductCode).Scan(&exists)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error checking duplicate")
        return
    }
    if exists {
        s.RespondWithError(w, http.StatusConflict, "DUPLICATE_CODE", "Product code already exists")
        return
    }

    query := `INSERT INTO products (company_id, product_code, product_name, description, 
                                    unit_price, cost_price, quantity_on_hand, minimum_stock, is_active) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
              RETURNING id, created_at, updated_at`
    
    err = s.DB.QueryRowContext(ctx, query, 
        product.CompanyID, product.ProductCode, product.ProductName,
        product.Description, product.UnitPrice, product.CostPrice, 
        product.QuantityOnHand, product.MinimumStock, product.IsActive).Scan(
        &product.ID, &product.CreatedAt, &product.UpdatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating product")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, product)
}

func (s *InventoryService) updateProductHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
    defer cancel()
    
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid product ID")
        return
    }
    
    var product Product
    if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }
    
    validator := validation.New()
    validator.Required("product_name", product.ProductName)
    
    if product.UnitPrice < 0 {
        validator.AddError("unit_price", "Unit price cannot be negative")
    }
    if product.CostPrice < 0 {
        validator.AddError("cost_price", "Cost price cannot be negative")
    }
    if product.MinimumStock < 0 {
        validator.AddError("minimum_stock", "Minimum stock cannot be negative")
    }
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `UPDATE products 
              SET product_name = $1, description = $2, unit_price = $3, cost_price = $4, 
                  minimum_stock = $5, is_active = $6, updated_at = CURRENT_TIMESTAMP 
              WHERE id = $7 AND company_id = $8 
              RETURNING updated_at`
    
    err = s.DB.QueryRowContext(ctx, query, product.ProductName, product.Description,
                              product.UnitPrice, product.CostPrice, product.MinimumStock, 
                              product.IsActive, id, companyID).Scan(&product.UpdatedAt)
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Product not found")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Error updating product")
        return
    }
    
    product.ID = id
    product.CompanyID = companyID
    s.RespondWithJSON(w, http.StatusOK, product)
}

func (s *InventoryService) deleteProductHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    vars := mux.Vars(r)
    id, err := strconv.Atoi(vars["id"])
    if err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_ID", "Invalid product ID")
        return
    }
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    // Soft delete by setting is_active to false
    query := `UPDATE products SET is_active = false, updated_at = CURRENT_TIMESTAMP 
              WHERE id = $1 AND company_id = $2`
    
    result, err := s.DB.ExecContext(ctx, query, id, companyID)
    if err != nil {
        s.HandleDBError(w, err, "Error deleting product")
        return
    }
    
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        s.RespondWithError(w, http.StatusNotFound, "NOT_FOUND", "Product not found")
        return
    }
    
    s.RespondWithJSON(w, http.StatusOK, map[string]string{
        "status": "deleted",
        "id":     strconv.Itoa(id),
    })
}

func (s *InventoryService) getStockMovementsHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    productID := r.URL.Query().Get("product_id")
    
    query := `SELECT sm.id, sm.company_id, sm.product_id, sm.movement_type, sm.quantity, 
                     sm.unit_cost, sm.reference_number, sm.movement_date, sm.notes, 
                     sm.created_by, sm.created_at
              FROM stock_movements sm WHERE sm.company_id = $1`
    
    args := []interface{}{companyID}
    
    if productID != "" {
        query += " AND sm.product_id = $2"
        args = append(args, productID)
    }
    
    query += " ORDER BY sm.movement_date DESC, sm.created_at DESC LIMIT 1000"
    
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching stock movements")
        return
    }
    defer rows.Close()
    
    var movements []StockMovement
    for rows.Next() {
        var movement StockMovement
        err := rows.Scan(&movement.ID, &movement.CompanyID, &movement.ProductID,
                        &movement.MovementType, &movement.Quantity, &movement.UnitCost,
                        &movement.ReferenceNumber, &movement.MovementDate, &movement.Notes,
                        &movement.CreatedBy, &movement.CreatedAt)
        if err != nil {
            continue
        }
        movements = append(movements, movement)
    }
    
    s.RespondWithJSON(w, http.StatusOK, movements)
}

func (s *InventoryService) createStockMovementHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()
    
    var movement StockMovement
    if err := json.NewDecoder(r.Body).Decode(&movement); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    if movement.ProductID == 0 {
        validator.AddError("product_id", "Product ID required")
    }
    validator.Required("movement_type", movement.MovementType)
    if movement.Quantity == 0 {
        validator.AddError("quantity", "Quantity cannot be zero")
    }

    validTypes := []string{"IN", "OUT", "ADJUSTMENT_IN", "ADJUSTMENT_OUT", "TRANSFER"}
    if !contains(validTypes, movement.MovementType) {
        validator.AddError("movement_type", "Invalid movement type")
    }

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    movement.CompanyID, _ = strconv.Atoi(r.Header.Get("Company-ID"))
    movement.CreatedBy, _ = strconv.Atoi(r.Header.Get("User-ID"))

    if movement.MovementDate.IsZero() {
        movement.MovementDate = time.Now()
    }

    tx, err := s.DB.BeginTx(ctx, nil)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Transaction failed")
        return
    }
    defer tx.Rollback()

    // Verify product exists and belongs to company
    var currentQty int
    err = tx.QueryRowContext(ctx, 
        "SELECT quantity_on_hand FROM products WHERE id = $1 AND company_id = $2 AND is_active = true",
        movement.ProductID, movement.CompanyID).Scan(&currentQty)
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_PRODUCT", "Product not found or inactive")
        return
    }
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error verifying product")
        return
    }

    // Check for negative stock on OUT movements
    var qtyChange int
    switch movement.MovementType {
    case "IN", "ADJUSTMENT_IN":
        qtyChange = movement.Quantity
    case "OUT", "ADJUSTMENT_OUT":
        qtyChange = -movement.Quantity
        if currentQty+qtyChange < 0 {
            s.RespondWithError(w, http.StatusBadRequest, "INSUFFICIENT_STOCK", 
                              "Insufficient stock for this movement")
            return
        }
    }

    // Create stock movement record
    query := `INSERT INTO stock_movements (company_id, product_id, movement_type, quantity, 
                                          unit_cost, reference_number, movement_date, notes, created_by) 
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
              RETURNING id, created_at`
    
    err = tx.QueryRowContext(ctx, query, 
        movement.CompanyID, movement.ProductID, movement.MovementType,
        movement.Quantity, movement.UnitCost, movement.ReferenceNumber, 
        movement.MovementDate, movement.Notes, movement.CreatedBy).Scan(&movement.ID, &movement.CreatedAt)
    if err != nil {
        s.HandleDBError(w, err, "Error creating stock movement")
        return
    }

    // Update product quantity
    _, err = tx.ExecContext(ctx, 
        "UPDATE products SET quantity_on_hand = quantity_on_hand + $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2", 
        qtyChange, movement.ProductID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error updating stock")
        return
    }

    if err = tx.Commit(); err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "COMMIT_ERROR", "Failed to commit")
        return
    }

    s.RespondWithJSON(w, http.StatusCreated, movement)
}

func (s *InventoryService) getLowStockHandler(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    companyID, _ := strconv.Atoi(r.Header.Get("Company-ID"))
    
    query := `SELECT id, company_id, product_code, product_name, description, 
                     unit_price, cost_price, quantity_on_hand, minimum_stock, 
                     is_active, created_at, updated_at
              FROM products 
              WHERE company_id = $1 AND is_active = true AND quantity_on_hand <= minimum_stock
              ORDER BY (quantity_on_hand - minimum_stock), product_name`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "DB_ERROR", "Error fetching low stock products")
        return
    }
    defer rows.Close()
    
    var products []Product
    for rows.Next() {
        var product Product
        err := rows.Scan(&product.ID, &product.CompanyID, &product.ProductCode, 
                        &product.ProductName, &product.Description, &product.UnitPrice, 
                        &product.CostPrice, &product.QuantityOnHand, &product.MinimumStock,
                        &product.IsActive, &product.CreatedAt, &product.UpdatedAt)
        if err != nil {
            continue
        }
        products = append(products, product)
    }
    
    s.RespondWithJSON(w, http.StatusOK, products)
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}