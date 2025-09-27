// api-gateway/main.go - ENHANCED VERSION WITH COMPANY SERVICE
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "strings"
    "sync"
    "time"
    
    "github.com/gorilla/mux"
    
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/middleware"
    "github.com/massehanto/accounting-system-go/shared/server"
)

type ServiceRegistry struct {
    services map[string]*ServiceConfig
    mutex    sync.RWMutex
    metrics  *ServiceMetrics
}

type ServiceConfig struct {
    URL         string
    HealthPath  string
    Timeout     time.Duration
    IsHealthy   bool
    LastCheck   time.Time
    FailCount   int
    MaxFails    int
    CircuitBreaker *CircuitBreaker
}

type CircuitBreaker struct {
    FailureThreshold int
    ResetTimeout     time.Duration
    State           string // "closed", "open", "half-open"
    LastFailTime    time.Time
    SuccessCount    int
    mutex           sync.RWMutex
}

type ServiceMetrics struct {
    RequestCount    map[string]int64
    ErrorCount      map[string]int64
    ResponseTimes   map[string][]time.Duration
    mutex           sync.RWMutex
}

func NewServiceRegistry() *ServiceRegistry {
    registry := &ServiceRegistry{
        services: map[string]*ServiceConfig{
            "user":         {getEnv("USER_SERVICE_URL", "http://localhost:8001"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "company":      {getEnv("COMPANY_SERVICE_URL", "http://localhost:8011"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "account":      {getEnv("ACCOUNT_SERVICE_URL", "http://localhost:8002"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "transaction":  {getEnv("TRANSACTION_SERVICE_URL", "http://localhost:8003"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "invoice":      {getEnv("INVOICE_SERVICE_URL", "http://localhost:8004"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "vendor":       {getEnv("VENDOR_SERVICE_URL", "http://localhost:8005"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "inventory":    {getEnv("INVENTORY_SERVICE_URL", "http://localhost:8006"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "report":       {getEnv("REPORT_SERVICE_URL", "http://localhost:8007"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "tax":          {getEnv("TAX_SERVICE_URL", "http://localhost:8008"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "currency":     {getEnv("CURRENCY_SERVICE_URL", "http://localhost:8009"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
            "notification": {getEnv("NOTIFICATION_SERVICE_URL", "http://localhost:8010"), "/health", 30 * time.Second, true, time.Now(), 0, 3, NewCircuitBreaker()},
        },
        metrics: NewServiceMetrics(),
    }
    
    go registry.startHealthChecking()
    go registry.startMetricsCollection()
    return registry
}

func NewCircuitBreaker() *CircuitBreaker {
    return &CircuitBreaker{
        FailureThreshold: 5,
        ResetTimeout:     60 * time.Second,
        State:           "closed",
        SuccessCount:    0,
    }
}

func NewServiceMetrics() *ServiceMetrics {
    return &ServiceMetrics{
        RequestCount:  make(map[string]int64),
        ErrorCount:    make(map[string]int64),
        ResponseTimes: make(map[string][]time.Duration),
    }
}

func (cb *CircuitBreaker) CanExecute() bool {
    cb.mutex.RLock()
    defer cb.mutex.RUnlock()
    
    switch cb.State {
    case "closed":
        return true
    case "open":
        return time.Since(cb.LastFailTime) > cb.ResetTimeout
    case "half-open":
        return true
    default:
        return false
    }
}

func (cb *CircuitBreaker) OnSuccess() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    if cb.State == "half-open" {
        cb.SuccessCount++
        if cb.SuccessCount >= 3 {
            cb.State = "closed"
            cb.SuccessCount = 0
        }
    }
}

func (cb *CircuitBreaker) OnFailure() {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.LastFailTime = time.Now()
    
    if cb.State == "closed" {
        cb.State = "open"
    } else if cb.State == "half-open" {
        cb.State = "open"
        cb.SuccessCount = 0
    }
}

func (sr *ServiceRegistry) startHealthChecking() {
    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        sr.checkServiceHealth()
    }
}

func (sr *ServiceRegistry) startMetricsCollection() {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        sr.rotateMetrics()
    }
}

func (sr *ServiceRegistry) rotateMetrics() {
    sr.metrics.mutex.Lock()
    defer sr.metrics.mutex.Unlock()
    
    // Keep only last 100 response times per service
    for service, times := range sr.metrics.ResponseTimes {
        if len(times) > 100 {
            sr.metrics.ResponseTimes[service] = times[len(times)-100:]
        }
    }
}

func (sr *ServiceRegistry) checkServiceHealth() {
    sr.mutex.Lock()
    defer sr.mutex.Unlock()
    
    for name, service := range sr.services {
        if !service.CircuitBreaker.CanExecute() {
            service.IsHealthy = false
            continue
        }
        
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        
        req, _ := http.NewRequestWithContext(ctx, "GET", service.URL+service.HealthPath, nil)
        client := &http.Client{Timeout: 5 * time.Second}
        
        start := time.Now()
        resp, err := client.Do(req)
        duration := time.Since(start)
        
        sr.recordMetric(name, duration, err == nil && resp != nil && resp.StatusCode == 200)
        
        if err != nil || resp == nil || resp.StatusCode != 200 {
            service.FailCount++
            service.CircuitBreaker.OnFailure()
            if service.FailCount >= service.MaxFails {
                service.IsHealthy = false
            }
            log.Printf("Health check failed for %s (attempt %d/%d): %v", name, service.FailCount, service.MaxFails, err)
        } else {
            service.IsHealthy = true
            service.FailCount = 0
            service.CircuitBreaker.OnSuccess()
            
            // Try to transition circuit breaker to half-open if it's open
            if service.CircuitBreaker.State == "open" && time.Since(service.CircuitBreaker.LastFailTime) > service.CircuitBreaker.ResetTimeout {
                service.CircuitBreaker.State = "half-open"
            }
        }
        
        service.LastCheck = time.Now()
        
        if resp != nil {
            resp.Body.Close()
        }
        cancel()
    }
}

func (sr *ServiceRegistry) recordMetric(serviceName string, duration time.Duration, success bool) {
    sr.metrics.mutex.Lock()
    defer sr.metrics.mutex.Unlock()
    
    sr.metrics.RequestCount[serviceName]++
    if !success {
        sr.metrics.ErrorCount[serviceName]++
    }
    
    if sr.metrics.ResponseTimes[serviceName] == nil {
        sr.metrics.ResponseTimes[serviceName] = make([]time.Duration, 0)
    }
    sr.metrics.ResponseTimes[serviceName] = append(sr.metrics.ResponseTimes[serviceName], duration)
}

func main() {
    cfg := config.Load()
    registry := NewServiceRegistry()
    
    r := mux.NewRouter()
    
    // Enhanced health check with service aggregation
    r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        registry.mutex.RLock()
        defer registry.mutex.RUnlock()
        
        allHealthy := true
        serviceStatus := make(map[string]interface{})
        
        for name, service := range registry.services {
            status := map[string]interface{}{
                "healthy":         service.IsHealthy,
                "last_check":      service.LastCheck.Format(time.RFC3339),
                "fail_count":      service.FailCount,
                "circuit_breaker": service.CircuitBreaker.State,
                "url":            service.URL,
            }
            
            // Add response time metrics
            registry.metrics.mutex.RLock()
            if times, exists := registry.metrics.ResponseTimes[name]; exists && len(times) > 0 {
                var total time.Duration
                for _, t := range times {
                    total += t
                }
                status["avg_response_time_ms"] = total.Milliseconds() / int64(len(times))
                status["request_count"] = registry.metrics.RequestCount[name]
                status["error_count"] = registry.metrics.ErrorCount[name]
                if registry.metrics.RequestCount[name] > 0 {
                    status["error_rate"] = float64(registry.metrics.ErrorCount[name]) / float64(registry.metrics.RequestCount[name])
                }
            }
            registry.metrics.mutex.RUnlock()
            
            serviceStatus[name] = status
            
            if !service.IsHealthy {
                allHealthy = false
            }
        }
        
        w.Header().Set("Content-Type", "application/json")
        if !allHealthy {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        
        response := map[string]interface{}{
            "status":    func() string { if allHealthy { return "healthy" } else { return "degraded" } }(),
            "gateway":   "api-gateway",
            "timestamp": time.Now().Format(time.RFC3339),
            "services":  serviceStatus,
            "version":   getEnv("VERSION", "1.0.0"),
            "environment": getEnv("GO_ENV", "development"),
        }
        
        json.NewEncoder(w).Encode(response)
    }).Methods("GET")
    
    // Metrics endpoint
    r.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
        registry.metrics.mutex.RLock()
        defer registry.metrics.mutex.RUnlock()
        
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "request_counts":  registry.metrics.RequestCount,
            "error_counts":    registry.metrics.ErrorCount,
            "timestamp":       time.Now().Format(time.RFC3339),
        })
    }).Methods("GET")
    
    setupRoutes(r, registry, cfg)
    server.SetupServer(r, cfg)
}

func setupRoutes(r *mux.Router, registry *ServiceRegistry, cfg *config.Config) {
    // Public routes with lighter middleware
    r.PathPrefix("/api/auth/").HandlerFunc(
        middleware.Chain(
            middleware.SecurityHeaders,
            middleware.RateLimit(20),
            middleware.LoggingMiddleware,
        )(createProxyHandler(registry, "user")))
    
    // API documentation
    r.PathPrefix("/api/docs").HandlerFunc(
        middleware.Chain(
            middleware.SecurityHeaders,
            middleware.LoggingMiddleware,
        )(serveAPIDocumentation))
    
    authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)
    
    // Protected routes with comprehensive middleware - UPDATED WITH COMPANY SERVICE
    protectedRoutes := map[string]string{
        "/api/users":           "user",
        "/api/companies":       "company",        // Fixed: Added company service routing
        "/api/accounts":        "account",
        "/api/ledger":          "account",
        "/api/transactions":    "transaction",
        "/api/invoices":        "invoice",
        "/api/customers":       "invoice",
        "/api/vendors":         "vendor",
        "/api/purchase-orders": "vendor",
        "/api/products":        "inventory",
        "/api/stock-movements": "inventory",
        "/api/low-stock":       "inventory",
        "/api/tax-rates":       "tax",
        "/api/calculate-tax":   "tax",
        "/api/convert":         "currency",
        "/api/rates":           "currency",
        "/api/reports":         "report",
        "/api/send-email":      "notification",
    }

    for path, service := range protectedRoutes {
        r.PathPrefix(path).HandlerFunc(
            middleware.Chain(
                middleware.SecurityHeaders,
                middleware.IndonesianComplianceHeaders,
                middleware.RateLimit(100),
                middleware.LoggingMiddleware,
                middleware.AuditLogger,
                authMiddleware,
            )(createProxyHandlerWithCircuitBreaker(registry, service)))
    }
}

func createProxyHandlerWithCircuitBreaker(registry *ServiceRegistry, serviceName string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        registry.mutex.RLock()
        service, exists := registry.services[serviceName]
        registry.mutex.RUnlock()
        
        if !exists {
            http.Error(w, fmt.Sprintf("Service %s not found", serviceName), http.StatusNotFound)
            return
        }
        
        // Check circuit breaker
        if !service.CircuitBreaker.CanExecute() {
            http.Error(w, fmt.Sprintf("Service %s is currently unavailable (circuit breaker open)", serviceName), 
                      http.StatusServiceUnavailable)
            return
        }
        
        if !service.IsHealthy {
            http.Error(w, fmt.Sprintf("Service %s is currently unhealthy", serviceName), 
                      http.StatusServiceUnavailable)
            return
        }
        
        targetURL, err := url.Parse(service.URL)
        if err != nil {
            http.Error(w, "Invalid service URL", http.StatusInternalServerError)
            return
        }
        
        proxy := httputil.NewSingleHostReverseProxy(targetURL)
        
        proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
            log.Printf("Proxy error for %s: %v", serviceName, err)
            
            // Record failure in circuit breaker
            service.CircuitBreaker.OnFailure()
            
            // Update service health
            registry.mutex.Lock()
            if svc, ok := registry.services[serviceName]; ok {
                svc.FailCount++
                if svc.FailCount >= svc.MaxFails {
                    svc.IsHealthy = false
                }
            }
            registry.mutex.Unlock()
            
            // Return appropriate error response
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusServiceUnavailable)
            
            response := map[string]interface{}{
                "error":     fmt.Sprintf("Service %s temporarily unavailable", serviceName),
                "code":      "SERVICE_UNAVAILABLE",
                "timestamp": time.Now().Format(time.RFC3339),
                "retry_after": "30",
            }
            json.NewEncoder(w).Encode(response)
        }
        
        // Modify request with enhanced headers
        originalPath := r.URL.Path
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        r.Header.Set("X-Forwarded-For", getClientIP(r))
        r.Header.Set("X-Gateway-Service", serviceName)
        r.Header.Set("X-Original-Path", originalPath)
        r.Header.Set("X-Request-Start", time.Now().Format(time.RFC3339Nano))
        r.Header.Set("X-Indonesian-Compliance", "enabled")
        
        // Record successful circuit breaker execution
        start := time.Now()
        proxy.ServeHTTP(w, r)
        duration := time.Since(start)
        
        // Record metrics
        registry.recordMetric(serviceName, duration, true)
        service.CircuitBreaker.OnSuccess()
    }
}

func createProxyHandler(registry *ServiceRegistry, serviceName string) http.HandlerFunc {
    return createProxyHandlerWithCircuitBreaker(registry, serviceName)
}

func serveAPIDocumentation(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    w.WriteHeader(http.StatusOK)
    
    html := `<!DOCTYPE html>
<html>
<head>
    <title>Indonesian Accounting System API</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui.css" />
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
        .header { background: #1976d2; color: white; padding: 1rem; text-align: center; }
        .container { max-width: 1200px; margin: 0 auto; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Indonesian Accounting System API Documentation</h1>
        <p>Professional accounting system for Indonesian businesses</p>
    </div>
    <div class="container">
        <div id="swagger-ui"></div>
    </div>
    <script src="https://unpkg.com/swagger-ui-dist@3.25.0/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/api-docs/openapi.yaml',
            dom_id: '#swagger-ui',
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIBundle.presets.standalone
            ],
            layout: "StandaloneLayout",
            deepLinking: true,
            showExtensions: true,
            showCommonExtensions: true
        });
    </script>
</body>
</html>`
    
    w.Write([]byte(html))
}

func getClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        ips := strings.Split(xff, ",")
        return strings.TrimSpace(ips[0])
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    ip := r.RemoteAddr
    if colon := strings.LastIndex(ip, ":"); colon != -1 {
        ip = ip[:colon]
    }
    return ip
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}