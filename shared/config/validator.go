// shared/config/validator.go
package config

import (
    "fmt"
    "os"
    "strings"
)

func ValidateEnvironment() error {
    required := map[string]ValidationRule{
        "JWT_SECRET":     {description: "JWT secret key", minLength: 32},
        "DB_PASSWORD":    {description: "Database password", minLength: 8},
        "SESSION_SECRET": {description: "Session secret key", minLength: 32},
    }
    
    var errors []string
    
    for key, rule := range required {
        value := os.Getenv(key)
        if value == "" {
            errors = append(errors, fmt.Sprintf("Missing required environment variable: %s (%s)", key, rule.description))
            continue
        }
        
        if len(value) < rule.minLength {
            errors = append(errors, fmt.Sprintf("Environment variable %s must be at least %d characters long, got %d", 
                                              key, rule.minLength, len(value)))
        }
    }
    
    // Validate optional but recommended variables
    optional := map[string]string{
        "SMTP_HOST":     "Email service configuration",
        "EXCHANGE_API_KEY": "Currency exchange rates",
        "REDIS_PASSWORD": "Redis cache security",
    }
    
    var warnings []string
    for key, description := range optional {
        if os.Getenv(key) == "" {
            warnings = append(warnings, fmt.Sprintf("Optional variable %s not set (%s)", key, description))
        }
    }
    
    if len(warnings) > 0 {
        fmt.Printf("Configuration warnings:\n- %s\n", strings.Join(warnings, "\n- "))
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("configuration validation failed:\n- %s", strings.Join(errors, "\n- "))
    }
    
    return nil
}

func ValidateBusinessRules() error {
    var errors []string
    
    // Validate Indonesian business requirements
    if currency := os.Getenv("DEFAULT_CURRENCY"); currency != "" && currency != "IDR" {
        errors = append(errors, "DEFAULT_CURRENCY should be IDR for Indonesian business compliance")
    }
    
    if timezone := os.Getenv("DEFAULT_TIMEZONE"); timezone != "" && timezone != "Asia/Jakarta" {
        errors = append(errors, "DEFAULT_TIMEZONE should be Asia/Jakarta for Indonesian business compliance")
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("business validation warnings:\n- %s", strings.Join(errors, "\n- "))
    }
    
    return nil
}

type ValidationRule struct {
    description string
    minLength   int
}

func ValidateAndLoad() *Config {
    if err := ValidateEnvironment(); err != nil {
        panic(err)
    }
    
    if err := ValidateBusinessRules(); err != nil {
        fmt.Printf("Warning: %v\n", err)
    }
    
    return Load()
}