// shared/middleware/security.go
package middleware

import (
    "net/http"
    "strings"
)

func EnhancedSecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Enhanced security headers for Indonesian compliance
        headers := map[string]string{
            // Basic security
            "X-Content-Type-Options":           "nosniff",
            "X-Frame-Options":                  "DENY",
            "X-XSS-Protection":                 "1; mode=block",
            "Referrer-Policy":                  "strict-origin-when-cross-origin",
            
            // Enhanced permissions policy
            "Permissions-Policy": "camera=(), microphone=(), geolocation=(), payment=(), " +
                                 "usb=(), magnetometer=(), gyroscope=(), accelerometer=(), " +
                                 "ambient-light-sensor=(), autoplay=(), encrypted-media=(), " +
                                 "fullscreen=(self), picture-in-picture=()",
            
            // Cross-origin policies
            "Cross-Origin-Embedder-Policy": "require-corp",
            "Cross-Origin-Opener-Policy":   "same-origin",
            "Cross-Origin-Resource-Policy": "same-origin",
            
            // Indonesian data protection compliance
            "X-Data-Jurisdiction":   "Indonesia",
            "X-Privacy-Policy":      "https://your-domain.co.id/privacy",
            "X-Terms-Of-Service":    "https://your-domain.co.id/terms",
            "X-Data-Protection":     "GDPR-compliant,Indonesia-compliant",
        }
        
        // Apply headers
        for key, value := range headers {
            w.Header().Set(key, value)
        }
        
        // HTTPS security
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", 
                          "max-age=31536000; includeSubDomains; preload")
        }
        
        // Content Security Policy based on request type
        if strings.HasPrefix(r.URL.Path, "/api") {
            // Strict CSP for API endpoints
            w.Header().Set("Content-Security-Policy", 
                          "default-src 'none'; frame-ancestors 'none'; "+
                          "base-uri 'none'; form-action 'none'")
        } else {
            // More permissive CSP for frontend
            csp := "default-src 'self'; " +
                   "script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
                   "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
                   "font-src 'self' https://fonts.gstatic.com; " +
                   "img-src 'self' data: https: blob:; " +
                   "connect-src 'self' https: wss: ws:; " +
                   "media-src 'self' blob: data:; " +
                   "object-src 'none'; " +
                   "base-uri 'self'; " +
                   "form-action 'self'; " +
                   "frame-ancestors 'none'; " +
                   "upgrade-insecure-requests"
            
            w.Header().Set("Content-Security-Policy", csp)
        }
        
        next(w, r)
    }
}

func CORSHeaders(allowedOrigins []string) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            
            // Check if origin is allowed
            allowed := false
            for _, allowedOrigin := range allowedOrigins {
                if allowedOrigin == "*" || allowedOrigin == origin {
                    allowed = true
                    break
                }
            }
            
            if allowed {
                w.Header().Set("Access-Control-Allow-Origin", origin)
                w.Header().Set("Access-Control-Allow-Credentials", "true")
                w.Header().Set("Access-Control-Allow-Methods", 
                              "GET, POST, PUT, DELETE, OPTIONS, PATCH")
                w.Header().Set("Access-Control-Allow-Headers", 
                              "Accept, Authorization, Content-Type, X-CSRF-Token, "+
                              "X-Requested-With, X-Request-ID, X-Company-ID, X-User-ID")
                w.Header().Set("Access-Control-Expose-Headers", 
                              "X-Request-ID, X-Response-Time, X-RateLimit-Remaining")
                w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
            }
            
            // Handle preflight requests
            if r.Method == http.MethodOptions {
                w.WriteHeader(http.StatusOK)
                return
            }
            
            next(w, r)
        }
    }
}

// Anti-CSRF protection
func CSRFProtection(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Skip CSRF for safe methods and API calls with proper headers
        if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
            next(w, r)
            return
        }
        
        // Check for API authentication header
        if auth := r.Header.Get("Authorization"); auth != "" && 
           strings.HasPrefix(auth, "Bearer ") {
            next(w, r)
            return
        }
        
        // Verify CSRF token for form submissions
        csrfToken := r.Header.Get("X-CSRF-Token")
        if csrfToken == "" {
            csrfToken = r.FormValue("csrf_token")
        }
        
        if csrfToken == "" {
            respondWithError(w, r, http.StatusForbidden, "CSRF_TOKEN_MISSING", 
                           "CSRF protection requires a token")
            return
        }
        
        // In a real implementation, you'd verify the token
        // For now, just check it's not empty
        if len(csrfToken) < 10 {
            respondWithError(w, r, http.StatusForbidden, "CSRF_TOKEN_INVALID", 
                           "Invalid CSRF token")
            return
        }
        
        next(w, r)
    }
}