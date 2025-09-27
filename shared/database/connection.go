// shared/database/connection.go - OPTIMIZED VERSION
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
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s "+
            "connect_timeout=10 statement_timeout=30000 "+
            "application_name=accounting-system",
        cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode)
    
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        log.Fatalf("Failed to create database connection: %v", err)
    }

    // Optimized connection pool settings
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    db.SetMaxIdleConns(cfg.MaxIdleConns)
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

    // Test connection with retry
    if err := testConnection(db); err != nil {
        db.Close()
        log.Fatalf("Database connection failed: %v", err)
    }

    // Set Indonesian configurations
    configureIndonesianSettings(db)

    log.Printf("Database connected: %s:%s/%s", cfg.Host, cfg.Port, cfg.Name)
    return db
}

func testConnection(db *sql.DB) error {
    for i := 0; i < 3; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        err := db.PingContext(ctx)
        cancel()
        
        if err == nil {
            return nil
        }
        
        if i < 2 {
            time.Sleep(time.Duration(i+1) * time.Second)
        }
    }
    
    return fmt.Errorf("database connection failed after 3 attempts")
}

func configureIndonesianSettings(db *sql.DB) {
    settings := []string{
        "SET timezone = 'Asia/Jakarta'",
        "SET datestyle = 'DMY'",
    }
    
    for _, setting := range settings {
        if _, err := db.Exec(setting); err != nil {
            log.Printf("Warning: Could not apply setting '%s': %v", setting, err)
        }
    }
}

func HealthCheck(db *sql.DB) error {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    
    var result int
    err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
    return err
}