// shared/middleware/middleware.go - SIMPLIFIED VERSION
package middleware

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
    
    "github.com/dgrijalva/jwt-go"
)

type Claims struct {
    UserID    int    `json:"user_id"`
    CompanyID int    `json:"company_id"`
    Role      string `json:"role"`
    jwt.StandardClaims
}

func SecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        next(w, r)
    }
}

func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next(w, r)
        log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
    }
}

func NewAuthMiddleware(jwtSecret string) func(http.HandlerFunc) http.HandlerFunc {
    jwtKey := []byte(jwtSecret)
    
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                respondWithError(w, http.StatusUnauthorized, "Authorization header required")
                return
            }

            if !strings.HasPrefix(authHeader, "Bearer ") {
                respondWithError(w, http.StatusUnauthorized, "Invalid authorization format")
                return
            }

            tokenString := strings.TrimPrefix(authHeader, "Bearer ")
            claims := &Claims{}
            
            token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
                return jwtKey, nil
            })

            if err != nil || !token.Valid {
                respondWithError(w, http.StatusUnauthorized, "Invalid token")
                return
            }

            // Add claims to request headers
            r.Header.Set("User-ID", fmt.Sprintf("%d", claims.UserID))
            r.Header.Set("Company-ID", fmt.Sprintf("%d", claims.CompanyID))
            r.Header.Set("User-Role", claims.Role)
            
            ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
            ctx = context.WithValue(ctx, "company_id", claims.CompanyID)
            
            next(w, r.WithContext(ctx))
        }
    }
}

func HealthCheck(db *sql.DB, serviceName string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        status := map[string]interface{}{
            "status":    "healthy",
            "service":   serviceName,
            "timestamp": time.Now().Format(time.RFC3339),
        }

        if db != nil {
            if err := db.Ping(); err != nil {
                status["status"] = "unhealthy"
                w.WriteHeader(http.StatusServiceUnavailable)
            }
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(status)
    }
}

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    
    response := map[string]interface{}{
        "error":     message,
        "timestamp": time.Now(),
    }
    
    json.NewEncoder(w).Encode(response)
}