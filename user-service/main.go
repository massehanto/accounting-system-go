// user-service/main.go - DECOUPLED VERSION
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

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type LoginResponse struct {
    Token string `json:"token"`
    User  User   `json:"user"`
}

func main() {
    cfg := config.Load()
    cfg.Database.Name = "user_db"
    
    db := database.InitDatabase(cfg.Database)
    defer db.Close()
    
    userService := &UserService{
        BaseService: &service.BaseService{DB: db},
        config:     cfg,
    }
    
    r := mux.NewRouter()
    
    r.Handle("/health", middleware.HealthCheck(db, "user-service")).Methods("GET")
    
    // Public endpoints
    r.Handle("/auth/login", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.LoggingMiddleware,
    )(userService.loginHandler)).Methods("POST")
    
    r.Handle("/auth/register", middleware.Chain(
        middleware.SecurityHeaders,
        middleware.LoggingMiddleware,
    )(userService.registerHandler)).Methods("POST")
    
    // Protected endpoints
    authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)
    r.Handle("/users", authMiddleware(userService.getUsersHandler)).Methods("GET")
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

    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    var user User
    var passwordHash string
    
    query := `SELECT id, email, password_hash, name, role, company_id, is_active, created_at
              FROM users WHERE LOWER(email) = LOWER($1) AND is_active = true`
    
    err := s.DB.QueryRowContext(ctx, query, req.Email).Scan(
        &user.ID, &user.Email, &passwordHash, &user.Name, 
        &user.Role, &user.CompanyID, &user.IsActive, &user.CreatedAt)
    
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Database error during login")
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
        s.RespondWithError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
        return
    }

    token, err := s.generateJWT(user)
    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Error generating token")
        return
    }

    // Update last login
    _, err = s.DB.ExecContext(ctx, "UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = $1", user.ID)
    if err != nil {
        // Log but don't fail
    }

    response := LoginResponse{
        Token: token,
        User:  user,
    }

    s.RespondWithJSON(w, http.StatusOK, response)
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
    validator.MinLength("password", req.Password, 8)
    validator.Required("name", req.Name)
    validator.MinLength("name", req.Name, 2)
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
        // Check if email exists
        var exists bool
        err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1))", req.Email).Scan(&exists)
        if err != nil {
            return err
        }
        if exists {
            s.RespondWithError(w, http.StatusConflict, "EMAIL_EXISTS", "Email already registered")
            return nil
        }

        // Hash password
        hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
        if err != nil {
            return err
        }

        // Create user
        query := `INSERT INTO users (email, password_hash, name, role, company_id, is_active) 
                  VALUES (LOWER($1), $2, $3, $4, $5, true) 
                  RETURNING id, created_at`
        
        var user User
        err = tx.QueryRow(query, req.Email, string(hashedPassword), req.Name, req.Role, req.CompanyID).Scan(
            &user.ID, &user.CreatedAt)
        if err != nil {
            return err
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
        s.RespondWithError(w, http.StatusInternalServerError, "REGISTRATION_ERROR", "Registration failed")
    }
}

func (s *UserService) getUsersHandler(w http.ResponseWriter, r *http.Request) {
    companyID := s.GetCompanyIDFromRequest(r)
    if companyID == 0 {
        s.RespondWithError(w, http.StatusBadRequest, "MISSING_COMPANY", "Company ID required")
        return
    }

    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    
    query := `SELECT id, email, name, role, company_id, is_active, last_login, created_at
              FROM users WHERE company_id = $1 ORDER BY created_at DESC`
    
    rows, err := s.DB.QueryContext(ctx, query, companyID)
    if err != nil {
        s.HandleDBError(w, err, "Error fetching users")
        return
    }
    defer rows.Close()
    
    var users []User
    for rows.Next() {
        var user User
        var lastLogin sql.NullTime
        err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Role, 
                        &user.CompanyID, &user.IsActive, &lastLogin, &user.CreatedAt)
        if err != nil {
            continue
        }
        if lastLogin.Valid {
            user.LastLogin = &lastLogin.Time
        }
        users = append(users, user)
    }
    
    s.RespondWithJSON(w, http.StatusOK, users)
}

func (s *UserService) getProfileHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    var user User
    query := `SELECT id, email, name, role, company_id, is_active, last_login, created_at
              FROM users WHERE id = $1`
    
    var lastLogin sql.NullTime
    err := s.DB.QueryRowContext(ctx, query, userID).Scan(
        &user.ID, &user.Email, &user.Name, &user.Role, 
        &user.CompanyID, &user.IsActive, &lastLogin, &user.CreatedAt)
    
    if err == sql.ErrNoRows {
        s.RespondWithError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
        return
    }
    if err != nil {
        s.HandleDBError(w, err, "Error fetching profile")
        return
    }

    if lastLogin.Valid {
        user.LastLogin = &lastLogin.Time
    }
    
    s.RespondWithJSON(w, http.StatusOK, user)
}

func (s *UserService) updateProfileHandler(w http.ResponseWriter, r *http.Request) {
    userID := s.GetUserIDFromRequest(r)
    
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
    validator.Required("email", req.Email)
    validator.Email("email", req.Email)

    if !validator.IsValid() {
        s.RespondValidationError(w, validator.Errors())
        return
    }

    err := s.WithTransaction(r.Context(), func(tx *sql.Tx) error {
        // Check email uniqueness
        var existingUserID int
        err := tx.QueryRow("SELECT id FROM users WHERE LOWER(email) = LOWER($1) AND id != $2", 
                          req.Email, userID).Scan(&existingUserID)
        if err != sql.ErrNoRows {
            if err == nil {
                s.RespondWithError(w, http.StatusConflict, "EMAIL_EXISTS", "Email already in use")
                return nil
            }
            return err
        }

        query := `UPDATE users SET name = $1, email = LOWER($2) WHERE id = $3`
        _, err = tx.Exec(query, req.Name, req.Email, userID)
        if err != nil {
            return err
        }

        response := map[string]interface{}{
            "name":  req.Name,
            "email": strings.ToLower(req.Email),
        }
        
        s.RespondWithJSON(w, http.StatusOK, response)
        return nil
    })

    if err != nil {
        s.RespondWithError(w, http.StatusInternalServerError, "UPDATE_ERROR", "Profile update failed")
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
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(s.config.JWT.Secret))
}