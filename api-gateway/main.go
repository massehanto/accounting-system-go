// api-gateway/main.go - SIMPLIFIED VERSION
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
    "strings"
    "time"
    
    "github.com/gorilla/mux"
    "github.com/rs/cors"
    "github.com/massehanto/accounting-system-go/shared/config"
    "github.com/massehanto/accounting-system-go/shared/middleware"
)

type ServiceConfig struct {
    URL string
}

func main() {
    cfg := config.Load()
    
    services := map[string]ServiceConfig{
        "user":         {getEnv("USER_SERVICE_URL", "http://localhost:8001")},
        "company":      {getEnv("COMPANY_SERVICE_URL", "http://localhost:8011")},
        "account":      {getEnv("ACCOUNT_SERVICE_URL", "http://localhost:8002")},
        "transaction":  {getEnv("TRANSACTION_SERVICE_URL", "http://localhost:8003")},
        "invoice":      {getEnv("INVOICE_SERVICE_URL", "http://localhost:8004")},
        "vendor":       {getEnv("VENDOR_SERVICE_URL", "http://localhost:8005")},
        "inventory":    {getEnv("INVENTORY_SERVICE_URL", "http://localhost:8006")},
        "report":       {getEnv("REPORT_SERVICE_URL", "http://localhost:8007")},
        "tax":          {getEnv("TAX_SERVICE_URL", "http://localhost:8008")},
        "currency":     {getEnv("CURRENCY_SERVICE_URL", "http://localhost:8009")},
        "notification": {getEnv("NOTIFICATION_SERVICE_URL", "http://localhost:8010")},
    }
    
    r := mux.NewRouter()
    
    // Health check
    r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "status":    "healthy",
            "gateway":   "api-gateway",
            "timestamp": time.Now().Format(time.RFC3339),
        })
    }).Methods("GET")
    
    // Route mapping
    routes := map[string]string{
        "/api/auth/":           "user",
        "/api/users":           "user",
        "/api/profile":         "user",
        "/api/companies":       "company",
        "/api/accounts":        "account",
        "/api/ledger":          "account",
        "/api/transactions":    "transaction",
        "/api/invoices":        "invoice",
        "/api/customers":       "invoice",
        "/api/vendors":         "vendor",
        "/api/purchase-orders": "vendor",
        "/api/products":        "inventory",
        "/api/stock-movements": "inventory",
        "/api/tax-rates":       "tax",
        "/api/calculate-tax":   "tax",
        "/api/convert":         "currency",
        "/api/rates":           "currency",
        "/api/reports":         "report",
        "/api/send-email":      "notification",
    }

    // Setup routes
    for path, serviceName := range routes {
        service := services[serviceName]
        r.PathPrefix(path).HandlerFunc(createProxyHandler(service.URL))
    }
    
    // CORS
    c := cors.New(cors.Options{
        AllowedOrigins:   cfg.CORS.AllowedOrigins,
        AllowedMethods:   cfg.CORS.AllowedMethods,
        AllowedHeaders:   cfg.CORS.AllowedHeaders,
        AllowCredentials: true,
    })
    
    handler := c.Handler(r)
    
    addr := fmt.Sprintf(":%s", cfg.Server.Port)
    log.Printf("🚀 API Gateway starting on %s", addr)
    log.Fatal(http.ListenAndServe(addr, handler))
}

func createProxyHandler(serviceURL string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        targetURL, err := url.Parse(serviceURL)
        if err != nil {
            http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
            return
        }
        
        proxy := httputil.NewSingleHostReverseProxy(targetURL)
        
        // Strip /api prefix
        r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
        
        proxy.ServeHTTP(w, r)
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}