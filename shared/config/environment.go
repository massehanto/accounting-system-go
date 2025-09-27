// shared/config/environment.go
package config

import (
    "fmt"
    "os"
    "regexp"
    "strconv"
    "strings"
)

type EnvironmentValidator struct {
    errors   []string
    warnings []string
}

func NewEnvironmentValidator() *EnvironmentValidator {
    return &EnvironmentValidator{
        errors:   make([]string, 0),
        warnings: make([]string, 0),
    }
}

func (ev *EnvironmentValidator) ValidateProduction() error {
    ev.validateJWTSecurity()
    ev.validateDatabaseSecurity()
    ev.validateSSLConfiguration()
    ev.validateServiceURLs()
    ev.validateIndonesianCompliance()
    ev.validateSecrets()
    ev.validateEmailConfiguration()
    
    if len(ev.errors) > 0 {
        return fmt.Errorf("production environment validation failed:\n- %s", 
                         strings.Join(ev.errors, "\n- "))
    }
    
    if len(ev.warnings) > 0 {
        fmt.Printf("Production warnings:\n- %s\n", 
                  strings.Join(ev.warnings, "\n- "))
    }
    
    return nil
}

func (ev *EnvironmentValidator) validateJWTSecurity() {
    jwtSecret := os.Getenv("JWT_SECRET")
    if len(jwtSecret) < 64 {
        ev.errors = append(ev.errors, 
            "JWT_SECRET must be at least 64 characters for production")
    }
    
    // Check for common weak patterns
    if strings.Contains(strings.ToLower(jwtSecret), "secret") ||
       strings.Contains(strings.ToLower(jwtSecret), "password") ||
       strings.Contains(strings.ToLower(jwtSecret), "key") {
        ev.warnings = append(ev.warnings, 
            "JWT_SECRET contains common words - consider using cryptographically random string")
    }
}

func (ev *EnvironmentValidator) validateDatabaseSecurity() {
    dbPassword := os.Getenv("DB_PASSWORD")
    if len(dbPassword) < 16 {
        ev.errors = append(ev.errors, 
            "DB_PASSWORD must be at least 16 characters for production")
    }
    
    sslMode := os.Getenv("DB_SSL_MODE")
    if sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
        ev.errors = append(ev.errors, 
            "DB_SSL_MODE must be 'require', 'verify-ca', or 'verify-full' for production")
    }
}

func (ev *EnvironmentValidator) validateSSLConfiguration() {
    nodeEnv := os.Getenv("NODE_ENV")
    goEnv := os.Getenv("GO_ENV")
    
    if nodeEnv == "production" || goEnv == "production" {
        if os.Getenv("SSL_CERT_PATH") == "" || os.Getenv("SSL_KEY_PATH") == "" {
            ev.warnings = append(ev.warnings, 
                "SSL certificate paths not configured for production")
        }
    }
}

func (ev *EnvironmentValidator) validateServiceURLs() {
    services := []string{
        "USER_SERVICE_URL", "ACCOUNT_SERVICE_URL", "TRANSACTION_SERVICE_URL",
        "INVOICE_SERVICE_URL", "VENDOR_SERVICE_URL", "INVENTORY_SERVICE_URL",
        "REPORT_SERVICE_URL", "TAX_SERVICE_URL", "CURRENCY_SERVICE_URL",
        "NOTIFICATION_SERVICE_URL",
    }
    
    httpsPattern := regexp.MustCompile(`^https://`)
    
    for _, service := range services {
        url := os.Getenv(service)
        if url != "" && !httpsPattern.MatchString(url) && 
           os.Getenv("GO_ENV") == "production" {
            ev.warnings = append(ev.warnings, 
                fmt.Sprintf("%s should use HTTPS in production", service))
        }
    }
}

func (ev *EnvironmentValidator) validateIndonesianCompliance() {
    currency := os.Getenv("DEFAULT_CURRENCY")
    if currency != "" && currency != "IDR" {
        ev.warnings = append(ev.warnings, 
            "DEFAULT_CURRENCY should be IDR for Indonesian compliance")
    }
    
    timezone := os.Getenv("DEFAULT_TIMEZONE")
    if timezone != "" && timezone != "Asia/Jakarta" {
        ev.warnings = append(ev.warnings, 
            "DEFAULT_TIMEZONE should be Asia/Jakarta for Indonesian compliance")
    }
    
    taxRate := os.Getenv("TAX_RATE_PPN")
    if taxRate != "" {
        rate, err := strconv.ParseFloat(taxRate, 64)
        if err != nil || rate != 11.0 {
            ev.warnings = append(ev.warnings, 
                "TAX_RATE_PPN should be 11.0 for current Indonesian PPN rate")
        }
    }
}

func (ev *EnvironmentValidator) validateSecrets() {
    sessionSecret := os.Getenv("SESSION_SECRET")
    if len(sessionSecret) < 64 {
        ev.errors = append(ev.errors, 
            "SESSION_SECRET must be at least 64 characters for production")
    }
    
    redisPassword := os.Getenv("REDIS_PASSWORD")
    if redisPassword == "" {
        ev.warnings = append(ev.warnings, 
            "REDIS_PASSWORD not set - Redis should be password protected in production")
    }
}

func (ev *EnvironmentValidator) validateEmailConfiguration() {
    smtpHost := os.Getenv("SMTP_HOST")
    smtpUser := os.Getenv("SMTP_USER")
    smtpPassword := os.Getenv("SMTP_PASSWORD")
    
    if smtpHost != "" && (smtpUser == "" || smtpPassword == "") {
        ev.warnings = append(ev.warnings, 
            "SMTP configuration incomplete - email notifications may fail")
    }
}

// Development environment validation
func (ev *EnvironmentValidator) ValidateDevelopment() error {
    ev.validateBasicRequired()
    ev.validateDevelopmentServices()
    
    if len(ev.errors) > 0 {
        return fmt.Errorf("development environment validation failed:\n- %s", 
                         strings.Join(ev.errors, "\n- "))
    }
    
    return nil
}

func (ev *EnvironmentValidator) validateBasicRequired() {
    required := map[string]string{
        "DB_PASSWORD":    "Database password",
        "JWT_SECRET":     "JWT secret key",
        "SESSION_SECRET": "Session secret key",
    }
    
    for key, description := range required {
        if os.Getenv(key) == "" {
            ev.errors = append(ev.errors, 
                fmt.Sprintf("%s (%s) is required", key, description))
        }
    }
}

func (ev *EnvironmentValidator) validateDevelopmentServices() {
    dbHost := os.Getenv("DB_HOST")
    if dbHost == "" {
        dbHost = "localhost"
    }
    
    if dbHost != "localhost" && dbHost != "127.0.0.1" && 
       !strings.HasPrefix(dbHost, "192.168.") {
        ev.warnings = append(ev.warnings, 
            "DB_HOST appears to be remote - ensure it's accessible")
    }
}