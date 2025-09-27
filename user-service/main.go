// user-service/main.go
package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "strings"
    "time"
    
    "github.com/gorilla/mux"
    _ "github.com/lib/pq"
    "golang.org/x/crypto/bcrypt"
    "github.com/dgrijalva/jwt-go"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/database"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
    "github.com/massehanto/accounting-system-go/shared/service"
    "github.com/massehanto/accounting-system-go/shared/validation"
)

type UserService struct {
    *service.BaseService
    config *config.Config
}

type User struct {
    ID        int       `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    Role      string    `json:"role"`
    CompanyID int       `json:"company_id"`
    IsActive  bool      `json:"is_active"`
    LastLogin *time.Time `json:"last_login,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}

type Company struct {
    ID      int    `json:"id"`
    Name    string `json:"name"`
    TaxID   string `json:"tax_id"`
    Address string `json:"address"`
    Phone   string `json:"phone"`
    Email   string `json:"email"`
}

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type LoginResponse struct {
    Token   string  `json:"token"`
    User    User    `json:"user"`
    Company Company `json:"company"`
}

func main() {
    cfg := config.ValidateAndLoad()
    cfg.Database.Name = "user_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    userService := &UserService{
        BaseService: &service.BaseService{DB: db},
        config:      cfg,
    }
    
    r := mux.NewRouter()
    
    // Health check
    r.Handle("/health", middleware.HealthCheck(db, "user-service")).Methods("GET")
    
    // Public endpoints
    r.Handle("/auth/login", middleware.PublicMiddleware()(userService.loginHandler)).Methods("POST")
    r.Handle("/auth/register", middleware.Chain(
        middleware.EnhancedSecurityHeaders,
        middleware.RateLimit(5),
        middleware.LoggingMiddleware,
    )(userService.registerHandler)).Methods("POST")
    
    // Protected endpoints
    authMiddleware := middleware.APIMiddleware(cfg.JWT.Secret)
    r.Handle("/users", authMiddleware(userService.getUsersHandler)).Methods("GET")
    r.Handle("/companies", authMiddleware(userService.getCompaniesHandler)).Methods("GET")
    r.Handle("/companies", authMiddleware(userService.createCompanyHandler)).Methods("POST")
    r.Handle("/auth/refresh", authMiddleware(userService.refreshTokenHandler)).Methods("POST")
    r.Handle("/profile", authMiddleware(userService.getProfileHandler)).Methods("GET")
    r.Handle("/profile", authMiddleware(userService.updateProfileHandler)).Methods("PUT")

    server.SetupServer(r, cfg)
}

func (s *UserService) loginHandler(w http.ResponseWriter, r *http.Request) {
    var req LoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("email", req.Email)
    validator.Email("email", req.Email)
    validator.Required("password", req.Password)
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        var user User
        var company Company
        var passwordHash string
        
        query := `SELECT u.id, u.email, u.password_hash, u.name, u.role, u.company_id, u.is_active, u.created_at,
                         c.name, c.tax_id, c.address, c.phone, c.email
                  FROM users u
                  JOIN companies c ON u.company_id = c.id
                  WHERE LOWER(u.email) = LOWER($1) AND u.is_active = true`
        
        err := s.DB.QueryRowContext(ctx, query, req.Email).Scan(
            &user.ID, &user.Email, &passwordHash, &user.Name, 
            &user.Role, &user.CompanyID, &user.IsActive, &user.CreatedAt,
            &company.Name, &company.TaxID, &company.Address, &company.Phone, &company.Email)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Database error during login")
            return nil
        }

        if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
            s.RespondWithError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
            return nil
        }

        token, err := s.generateJWT(user)
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Error generating authentication token")
            return nil
        }

        // Update last login timestamp
        _, err = s.DB.ExecContext(ctx, "UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = $1", user.ID)
        if err != nil {
            // Log error but don't fail the login
            return nil
        }

        company.ID = user.CompanyID
        response := LoginResponse{
            Token:   token,
            User:    user,
            Company: company,
        }

        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "LOGIN_ERROR", "Login process failed")
    }
}

func (s *UserService) registerHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email     string `json:"email"`
        Password  string `json:"password"`
        Name      string `json:"name"`
        Role      string `json:"role"`
        CompanyID int    `json:"company_id"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("email", req.Email)
    validator.Email("email", req.Email)
    validator.Required("password", req.Password)
    validator.StrongPassword("password", req.Password)
    validator.Required("name", req.Name)
    validator.MinLength("name", req.Name, 2)
    validator.MaxLength("name", req.Name, 100)
    validator.Required("role", req.Role)
    
    validRoles := []string{"admin", "manager", "accountant", "user"}
    validator.OneOf("role", req.Role, validRoles)
    
    if req.CompanyID == 0 {
        validator.AddError("company_id", "Company ID is required")
    }
    
    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check if email already exists
        var exists bool
        err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))", req.Email).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "EMAIL_EXISTS", "Email address is already registered")
            return nil
        }

        // Verify company exists
        var companyExists bool
        err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)", req.CompanyID).Scan(&companyExists)
        if err != nil {
            return err
        }
        if !companyExists {
            s.RespondWithError(w, http.StatusBadRequest, "INVALID_COMPANY", "Invalid company ID")
            return nil
        }

        // Hash password
        hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.config.Security.BCryptCost)
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "HASH_ERROR", "Error processing password")
            return nil
        }

        // Create user
        query := `INSERT INTO users (email, password_hash, name, role, company_id, is_active) 
                  VALUES (LOWER($1), $2, $3, $4, $5, true) 
                  RETURNING id, created_at`
        
        var user User
        err = tx.QueryRow(query, req.Email, string(hashedPassword), req.Name, req.Role, req.CompanyID).Scan(&user.ID, &user.CreatedAt)
        if err != nil {
            s.HandleDBError(w, err, "Error creating user account")
            return nil
        }

        user.Email = strings.ToLower(req.Email)
        user.Name = req.Name
        user.Role = req.Role
        user.CompanyID = req.CompanyID
        user.IsActive = true

        s.RespondWithJSON(w, http.StatusCreated, user)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REGISTRATION_ERROR", "Registration process failed")
    }
}

func (s *UserService) getUsersHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID is required")
        return
    }

    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        query := `SELECT id, email, name, role, company_id, is_active, last_login, created_at
                  FROM users 
                  WHERE company_id = $1
                  ORDER BY created_at DESC`
        
        rows, err := s.DB.QueryContext(ctx, query, companyID)
        if err != nil {
            s.HandleDBError(w, err, "Error fetching users")
            return nil
        }
        defer rows.Close()
        
        var users []User
        for rows.Next() {
            var user User
            var lastLogin sql.NullTime
            err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.CompanyID,
                            &user.IsActive, &lastLogin, &user.CreatedAt)
            if err != nil {
                continue
            }
            if lastLogin.Valid {
                user.LastLogin = &lastLogin.Time
            }
            users = append(users, user)
        }
        
        s.RespondWithJSON(w, http.StatusOK, users)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "FETCH_ERROR", "Error retrieving users")
    }
}

func (s *UserService) getCompaniesHandler(w http.ResponseWriter, r *http.Request) {
    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        query := `SELECT id, name, tax_id, address, phone, email 
                  FROM companies 
                  ORDER BY name`
        
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
                            &company.Phone, &company.Email)
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

func (s *UserService) createCompanyHandler(w http.ResponseWriter, r *http.Request) {
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
                  RETURNING id`
        
        err = tx.QueryRow(query, company.Name, company.TaxID, company.Address,
                         company.Phone, company.Email).Scan(&company.ID)
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

func (s *UserService) getProfileHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    if userID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_USER", "User ID is required")
        return
    }

    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        var user User
        var company Company
        
        query := `SELECT u.id, u.email, u.name, u.role, u.company_id, u.is_active, u.last_login, u.created_at,
                         c.name, c.tax_id, c.address, c.phone, c.email
                  FROM users u
                  JOIN companies c ON u.company_id = c.id
                  WHERE u.id = $1`
        
        var lastLogin sql.NullTime
        err := s.DB.QueryRowContext(ctx, query, userID).Scan(
            &user.ID, &user.Email, &user.Name, &user.Role, 
            &user.CompanyID, &user.IsActive, &lastLogin, &user.CreatedAt,
            &company.Name, &company.TaxID, &company.Address, &company.Phone, &company.Email)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "USER_NOT_FOUND", "User profile not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error fetching user profile")
            return nil
        }

        if lastLogin.Valid {
            user.LastLogin = &lastLogin.Time
        }
        
        company.ID = user.CompanyID
        
        response := map[string]interface{}{
            "user":    user,
            "company": company,
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "PROFILE_ERROR", "Error retrieving profile")
    }
}

func (s *UserService) updateProfileHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    if userID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_USER", "User ID is required")
        return
    }

    var req struct {
        Name  string `json:"name"`
        Email string `json:"email"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.RespondWithError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
        return
    }

    validator := validation.New()
    validator.Required("name", req.Name)
    validator.MinLength("name", req.Name, 2)
    validator.MaxLength("name", req.Name, 100)
    validator.Required("email", req.Email)
    validator.Email("email", req.Email)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check if email is already taken by another user
        var existingUserID int
        err := tx.QueryRow("SELECT id FROM users WHERE LOWER(email) = LOWER($1) AND id != $2", req.Email, userID).Scan(&existingUserID)
        if err != sql.ErrNoRows {
            if err == nil {
                s.RespondWithError(w, http.StatusConflict, "EMAIL_EXISTS", "Email address is already in use")
                return nil
            }
            return err
        }

        query := `UPDATE users 
                  SET name = $1, email = LOWER($2), updated_at = CURRENT_TIMESTAMP 
                  WHERE id = $3
                  RETURNING email, name, updated_at`
        
        var updatedUser struct {
            Email     string    `json:"email"`
            Name      string    `json:"name"`
            UpdatedAt time.Time `json:"updated_at"`
        }
        
        err = tx.QueryRow(query, req.Name, req.Email, userID).Scan(&updatedUser.Email, &updatedUser.Name, &updatedUser.UpdatedAt)
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error updating profile")
            return nil
        }

        s.RespondWithJSON(w, http.StatusOK, updatedUser)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "UPDATE_ERROR", "Profile update failed")
    }
}

func (s *UserService) refreshTokenHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    if userID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_USER", "User ID is required")
        return
    }

    err := s.ExecuteWithTimeout(5*time.Second, func(ctx context.Context) error {
        var user User
        query := `SELECT id, email, name, role, company_id, is_active 
                  FROM users 
                  WHERE id = $1 AND is_active = true`
        
        err := s.DB.QueryRowContext(ctx, query, userID).Scan(
            &user.ID, &user.Email, &user.Name, &user.Role, &user.CompanyID, &user.IsActive)
        
        if err == sql.ErrNoRows {
            s.RespondWithError(w, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found or inactive")
            return nil
        }
        if err != nil {
            s.HandleDBError(w, err, "Error refreshing token")
            return nil
        }

        token, err := s.generateJWT(user)
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Error generating new token")
            return nil
        }

        response := map[string]interface{}{
            "token": token,
            "user":  user,
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "REFRESH_ERROR", "Token refresh failed")
    }
}

func (s *UserService) generateJWT(user User) (string, error) {
    expirationTime := time.Now().Add(s.config.JWT.Expiration)
    claims := &middleware.Claims{
        UserID:    user.ID,
        CompanyID: user.CompanyID,
        Role:      user.Role,
        StandardClaims: jwt.StandardClaims{
            ExpiresAt: expirationTime.Unix(),
            IssuedAt:  time.Now().Unix(),
            Subject:   user.Email,
            Issuer:    s.config.JWT.Issuer,
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.config.JWT.Secret))
}