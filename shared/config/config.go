// shared/config/config.go - CONSISTENT VERSION
package config

import (
    "fmt"
    "log"
    "os"
    "strconv"
    "time"
)

type Config struct {
    Database   DatabaseConfig
    Server     ServerConfig
    JWT        JWTConfig
    CORS       CORSConfig
    Security   SecurityConfig
    Business   BusinessConfig
}

type DatabaseConfig struct {
    Host            string
    Port            string
    User            string
    Password        string
    Name            string
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
    ConnMaxIdleTime time.Duration
    SSLMode         string
}

type ServerConfig struct {
    Port         string
    Host         string
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    IdleTimeout  time.Duration
}

type JWTConfig struct {
    Secret     string
    Expiration time.Duration
    Issuer     string
}

type CORSConfig struct {
    AllowedOrigins []string
    AllowedMethods []string
    AllowedHeaders []string
}

type SecurityConfig struct {
    BCryptCost     int
    SessionSecret  string
}

type BusinessConfig struct {
    DefaultCurrency string
    DefaultTimezone string
    TaxRatePPN      float64
}

// Main config loader with validation
func Load() *Config {
    if err := validateRequired(); err != nil {
        log.Fatalf("Configuration validation failed: %v", err)
    }
    
    return &Config{
        Database: DatabaseConfig{
            Host:            getEnv("DB_HOST", "localhost"),
            Port:            getEnv("DB_PORT", "5432"),
            User:            getEnv("DB_USER", "postgres"),
            Password:        getEnvRequired("DB_PASSWORD"),
            Name:            getEnv("DB_NAME", ""),
            MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
            MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
            ConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME", 300)) * time.Second,
            ConnMaxIdleTime: time.Duration(getEnvInt("DB_CONN_MAX_IDLE", 60)) * time.Second,
            SSLMode:         getEnv("DB_SSL_MODE", "disable"),
        },
        Server: ServerConfig{
            Port:         getEnv("PORT", "8000"),
            Host:         getEnv("HOST", "0.0.0.0"),
            ReadTimeout:  30 * time.Second,
            WriteTimeout: 30 * time.Second,
            IdleTimeout:  60 * time.Second,
        },
        JWT: JWTConfig{
            Secret:     getEnvRequired("JWT_SECRET"),
            Expiration: time.Duration(getEnvInt("JWT_EXPIRATION", 86400)) * time.Second,
            Issuer:     getEnv("JWT_ISSUER", "accounting-system"),
        },
        CORS: CORSConfig{
            AllowedOrigins: []string{getEnv("FRONTEND_URL", "http://localhost:3000")},
            AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
            AllowedHeaders: []string{"*"},
        },
        Security: SecurityConfig{
            BCryptCost:    getEnvInt("BCRYPT_COST", 12),
            SessionSecret: getEnvRequired("SESSION_SECRET"),
        },
        Business: BusinessConfig{
            DefaultCurrency: getEnv("DEFAULT_CURRENCY", "IDR"),
            DefaultTimezone: getEnv("DEFAULT_TIMEZONE", "Asia/Jakarta"),
            TaxRatePPN:      getEnvFloat("TAX_RATE_PPN", 11.00),
        },
    }
}

// Validate required environment variables
func validateRequired() error {
    required := []string{"JWT_SECRET", "DB_PASSWORD", "SESSION_SECRET"}
    
    for _, key := range required {
        if os.Getenv(key) == "" {
            return fmt.Errorf("required environment variable %s is not set", key)
        }
    }
    
    // Validate JWT secret length
    if len(os.Getenv("JWT_SECRET")) < 32 {
        return fmt.Errorf("JWT_SECRET must be at least 32 characters long")
    }
    
    return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvRequired(key string) string {
    value := os.Getenv(key)
    if value == "" {
        log.Fatalf("Required environment variable %s is not set", key)
    }
    return value
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
    if value := os.Getenv(key); value != "" {
        if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
            return floatValue
        }
    }
    return defaultValue
}