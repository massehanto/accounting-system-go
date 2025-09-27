// shared/middleware/middleware.go - CLEANED UP VERSION
package middleware

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"
    
    "github.com/dgrijalva/jwt-go"
    "golang.org/x/time/rate"
)

type Claims struct {
    UserID    int    `json:"user_id"`
    CompanyID int    `json:"company_id"`
    Role      string `json:"role"`
    jwt.StandardClaims
}

type ErrorResponse struct {
    Error     string      `json:"error"`
    Code      string      `json:"code,omitempty"`
    Details   interface{} `json:"details,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
    RequestID string      `json:"request_id,omitempty"`
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
    size       int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
    if rw.statusCode == 0 {
        rw.statusCode = http.StatusOK
    }
    size, err := rw.ResponseWriter.Write(b)
    rw.size += size
    return size, err
}

// Authentication Middleware
func NewAuthMiddleware(jwtSecret string) func(http.HandlerFunc) http.HandlerFunc {
    if len(jwtSecret) < 32 {
        panic("JWT secret must be at least 32 characters long")
    }
    
    jwtKey := []byte(jwtSecret)
    
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                respondWithError(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Authorization header required")
                return
            }

            if !strings.HasPrefix(authHeader, "Bearer ") {
                respondWithError(w, r, http.StatusUnauthorized, "INVALID_AUTH_FORMAT", "Authorization must be Bearer token")
                return
            }

            tokenString := strings.TrimPrefix(authHeader, "Bearer ")
            if len(tokenString) == 0 {
                respondWithError(w, r, http.StatusUnauthorized, "MISSING_TOKEN", "Token is required")
                return
            }

            claims := &Claims{}
            token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
                if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                    return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
                }
                return jwtKey, nil
            })

            if err != nil {
                if ve, ok := err.(*jwt.ValidationError); ok {
                    if ve.Errors&jwt.ValidationErrorExpired != 0 {
                        respondWithError(w, r, http.StatusUnauthorized, "TOKEN_EXPIRED", "Token has expired")
                        return
                    }
                    if ve.Errors&jwt.ValidationErrorSignatureInvalid != 0 {
                        respondWithError(w, r, http.StatusUnauthorized, "INVALID_SIGNATURE", "Invalid token signature")
                        return
                    }
                }
                respondWithError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid token")
                return
            }

            if !token.Valid {
                respondWithError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "Token is not valid")
                return
            }

            // Enhanced security: validate token timing
            now := time.Now()
            if claims.ExpiresAt > 0 && now.Unix() > claims.ExpiresAt {
                respondWithError(w, r, http.StatusUnauthorized, "TOKEN_EXPIRED", "Token has expired")
                return
            }

            // Validate issuer if set
            if claims.Issuer != "" && claims.Issuer != "accounting-system" {
                respondWithError(w, r, http.StatusUnauthorized, "INVALID_ISSUER", "Invalid token issuer")
                return
            }

            // Add claims to request headers and context
            r.Header.Set("User-ID", fmt.Sprintf("%d", claims.UserID))
            r.Header.Set("Company-ID", fmt.Sprintf("%d", claims.CompanyID))
            r.Header.Set("User-Role", claims.Role)
            
            ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
            ctx = context.WithValue(ctx, "company_id", claims.CompanyID)
            ctx = context.WithValue(ctx, "user_role", claims.Role)
            ctx = context.WithValue(ctx, "claims", claims)
            
            next(w, r.WithContext(ctx))
        }
    }
}

// Logging Middleware
func LoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        requestID := generateRequestID()
        r.Header.Set("X-Request-ID", requestID)
        w.Header().Set("X-Request-ID", requestID)
        
        wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        
        next(wrapped, r)
        
        duration := time.Since(start)
        
        logData := map[string]interface{}{
            "request_id": requestID,
            "method":     r.Method,
            "path":       r.URL.Path,
            "status":     wrapped.statusCode,
            "duration_ms": duration.Milliseconds(),
            "size":       wrapped.size,
            "user_agent": r.UserAgent(),
            "remote_addr": getClientIP(r),
            "timestamp":  start.Format(time.RFC3339),
        }
        
        if userID := r.Header.Get("User-ID"); userID != "" {
            logData["user_id"] = userID
        }
        if companyID := r.Header.Get("Company-ID"); companyID != "" {
            logData["company_id"] = companyID
        }
        
        logLevel := getLogLevel(wrapped.statusCode, duration)
        log.Printf("[%s] %+v", logLevel, logData)
    }
}

// Indonesian Compliance Headers
func IndonesianComplianceHeaders(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Indonesian data localization headers
        w.Header().Set("X-Data-Location", "ID")
        w.Header().Set("X-Jurisdiction", "Indonesia")
        w.Header().Set("X-Timezone", "Asia/Jakarta")
        w.Header().Set("X-Currency", "IDR")
        
        // Indonesian language preference
        if acceptLang := r.Header.Get("Accept-Language"); acceptLang == "" {
            w.Header().Set("Content-Language", "id-ID")
        }
        
        next(w, r)
    }
}

// Basic Security Headers
func SecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Basic security headers
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        
        // Enhanced security headers
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), magnetometer=(), gyroscope=()")
        w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
        w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
        w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
        
        // HTTPS security
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
        }
        
        // Content Security Policy tailored for API endpoints
        if strings.HasPrefix(r.URL.Path, "/api") {
            w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
        } else {
            csp := "default-src 'self'; " +
                   "script-src 'self' 'unsafe-inline'; " +
                   "style-src 'self' 'unsafe-inline'; " +
                   "img-src 'self' data: https:; " +
                   "connect-src 'self'; " +
                   "font-src 'self'; " +
                   "frame-ancestors 'none'"
            w.Header().Set("Content-Security-Policy", csp)
        }
        
        next(w, r)
    }
}

// Rate Limiting Middleware
func RateLimit(requestsPerMinute int) func(http.HandlerFunc) http.HandlerFunc {
    limiters := make(map[string]*rate.Limiter)
    
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            ip := getClientIP(r)
            
            limiter, exists := limiters[ip]
            if !exists {
                limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), requestsPerMinute)
                limiters[ip] = limiter
            }
            
            if !limiter.Allow() {
                w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
                w.Header().Set("X-RateLimit-Remaining", "0")
                w.Header().Set("Retry-After", "60")
                respondWithError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", 
                    "Too many requests. Please try again later.")
                return
            }
            
            next(w, r)
        }
    }
}

// Audit Logger
func AuditLogger(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Enhanced audit logging for Indonesian compliance
        auditData := map[string]interface{}{
            "timestamp":   time.Now().Format(time.RFC3339),
            "method":      r.Method,
            "path":        r.URL.Path,
            "remote_addr": getClientIP(r),
            "user_agent":  r.UserAgent(),
        }
        
        if userID := r.Header.Get("User-ID"); userID != "" {
            auditData["user_id"] = userID
        }
        if companyID := r.Header.Get("Company-ID"); companyID != "" {
            auditData["company_id"] = companyID
        }
        
        // Log sensitive operations
        if r.Method != "GET" || strings.Contains(r.URL.Path, "auth") {
            log.Printf("AUDIT: %+v", auditData)
        }
        
        next(w, r)
    }
}

// Health Check Handler
func HealthCheck(db *sql.DB, serviceName string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        defer cancel()
        
        status := map[string]interface{}{
            "status":    "healthy",
            "service":   serviceName,
            "timestamp": time.Now().Format(time.RFC3339),
            "version":   "1.0.0",
            "database":  "connected",
            "timezone":  "Asia/Jakarta",
            "currency":  "IDR",
        }

        if db != nil {
            if err := db.PingContext(ctx); err != nil {
                status["status"] = "unhealthy"
                status["database"] = "disconnected"
                status["error"] = "Database connection failed"
                w.WriteHeader(http.StatusServiceUnavailable)
            }
        } else {
            status["database"] = "not_applicable"
        }

        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Cache-Control", "no-cache")
        json.NewEncoder(w).Encode(status)
    }
}

// Utility functions
func respondWithError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    
    response := ErrorResponse{
        Error:     message,
        Code:      code,
        Timestamp: time.Now(),
        RequestID: r.Header.Get("X-Request-ID"),
    }
    
    json.NewEncoder(w).Encode(response)
}

func getClientIP(r *http.Request) string {
    // Check X-Forwarded-For header
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        ips := strings.Split(xff, ",")
        return strings.TrimSpace(ips[0])
    }
    
    // Check X-Real-IP header
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    
    // Fallback to RemoteAddr
    ip := r.RemoteAddr
    if colon := strings.LastIndex(ip, ":"); colon != -1 {
        ip = ip[:colon]
    }
    return ip
}

func getLogLevel(statusCode int, duration time.Duration) string {
    if statusCode >= 500 {
        return "ERROR"
    }
    if statusCode >= 400 {
        return "WARN"
    }
    if duration > 5*time.Second {
        return "SLOW"
    }
    return "INFO"
}

func generateRequestID() string {
    return fmt.Sprintf("%d", time.Now().UnixNano())
}