// user-service/main.go - REFACTORED FOR PROPER SERVICE SEPARATION
package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "net/http"
    "strings"
    "time"
    "bytes"
    "fmt"
    "os"
    
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
    config            *config.Config
    companyServiceURL string
    client            *http.Client
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
        BaseService:       &service.BaseService{DB: db},
        config:           cfg,
        companyServiceURL: getEnv("COMPANY_SERVICE_URL", "http://localhost:8011"),
        client:           &http.Client{Timeout: 30 * time.Second},
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
        var passwordHash string
        
        // Only get user data from user service
        query := `SELECT id, email, password_hash, name, role, company_id, is_active, created_at
                  FROM users WHERE LOWER(email) = LOWER($1) AND is_active = true`
        
        err := s.DB.QueryRowContext(ctx, query, req.Email).Scan(
            &user.ID, &user.Email, &passwordHash, &user.Name, 
            &user.Role, &user.CompanyID, &user.IsActive, &user.CreatedAt)
        
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

        // Fetch company data from company service
        company, err := s.fetchCompanyData(r.Context(), user.CompanyID, r.Header.Get("Authorization"))
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "COMPANY_ERROR", "Error fetching company data")
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

        response := LoginResponse{
            Token:   token,
            User:    user,
            Company: *company,
        }

        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "LOGIN_ERROR", "Login process failed")
    }
}

// Fetch company data from company service
func (s *UserService) fetchCompanyData(ctx context.Context, companyID int, authHeader string) (*Company, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", 
        fmt.Sprintf("%s/companies/%d", s.companyServiceURL, companyID), nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", authHeader)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := s.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("company service error: %d", resp.StatusCode)
    }
    
    var response struct {
        Data Company `json:"data"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, err
    }
    
    return &response.Data, nil
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

        // Verify company exists via company service
        _, err = s.fetchCompanyData(r.Context(), req.CompanyID, r.Header.Get("Authorization"))
        if err != nil {
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

func (s *UserService) getProfileHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    if userID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_USER", "User ID is required")
        return
    }

    err := s.ExecuteWithTimeout(10*time.Second, func(ctx context.Context) error {
        var user User
        
        query := `SELECT id, email, name, role, company_id, is_active, last_login, created_at
                  FROM users WHERE id = $1`
        
        var lastLogin sql.NullTime
        err := s.DB.QueryRowContext(ctx, query, userID).Scan(
            &user.ID, &user.Email, &user.Name, &user.Role, 
            &user.CompanyID, &user.IsActive, &lastLogin, &user.CreatedAt)
        
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
        
        // Fetch company data from company service
        company, err := s.fetchCompanyData(r.Context(), user.CompanyID, r.Header.Get("Authorization"))
        if err != nil {
            s.RespondWithError(w, http.StatusInternalServerError, "COMPANY_ERROR", "Error fetching company data")
            return nil
        }
        
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

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}