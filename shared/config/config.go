// shared/config/config.go - SIMPLIFIED VERSION
package config

import (
    "fmt"
    "log"
    "os"
    "strconv"
    "time"
)

type Config struct {
    Database DatabaseConfig
    Server   ServerConfig
    JWT      JWTConfig
    CORS     CORSConfig
}

type DatabaseConfig struct {
    Host     string
    Port     string
    User     string
    Password string
    Name     string
    SSLMode  string
}

type ServerConfig struct {
    Port string
    Host string
}

type JWTConfig struct {
    Secret     string
    Expiration time.Duration
}

type CORSConfig struct {
    AllowedOrigins []string
    AllowedMethods []string
    AllowedHeaders []string
}

func Load() *Config {
    // Validate required environment variables
    required := []string{"JWT_SECRET", "DB_PASSWORD"}
    for _, key := range required {
        if os.Getenv(key) == "" {
            log.Fatalf("Required environment variable %s is not set", key)
        }
    }
    
    if len(os.Getenv("JWT_SECRET")) < 32 {
        log.Fatalf("JWT_SECRET must be at least 32 characters long")
    }
    
    return &Config{
        Database: DatabaseConfig{
            Host:     getEnv("DB_HOST", "localhost"),
            Port:     getEnv("DB_PORT", "5432"),
            User:     getEnv("DB_USER", "postgres"),
            Password: os.Getenv("DB_PASSWORD"),
            Name:     getEnv("DB_NAME", ""),
            SSLMode:  getEnv("DB_SSL_MODE", "disable"),
        },
        Server: ServerConfig{
            Port: getEnv("PORT", "8000"),
            Host: getEnv("HOST", "0.0.0.0"),
        },
        JWT: JWTConfig{
            Secret:     os.Getenv("JWT_SECRET"),
            Expiration: time.Duration(getEnvInt("JWT_EXPIRATION", 86400)) * time.Second,
        },
        CORS: CORSConfig{
            AllowedOrigins: []string{getEnv("FRONTEND_URL", "http://localhost:3000")},
            AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
            AllowedHeaders: []string{"*"},
        },
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}