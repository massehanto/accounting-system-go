// shared/database/connection.go - SIMPLIFIED VERSION
package database

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    "time"
    
    _ "github.com/lib/pq"
    "github.com/massehanto/accounting-system-go/shared/config"
)

func InitDatabase(cfg config.DatabaseConfig) *sql.DB {
    dsn := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)
    
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatalf("Failed to create database connection: %v", err)
    }

    // Basic connection pool settings
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    // Test connection
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := db.PingContext(ctx); err != nil {
        db.Close()
        log.Fatalf("Database connection failed: %v", err)
    }

    log.Printf("Database connected: %s:%s/%s", cfg.Host, cfg.Port, cfg.Name)
    return db
}

func HealthCheck(db *sql.DB) error {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    
    return db.PingContext(ctx)
}