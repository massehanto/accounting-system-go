package server

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gorilla/mux"
    "github.com/rs/cors"
    
    "github.com/massehanto/accounting-system-go/shared/config"
)

func SetupServer(r *mux.Router, cfg *config.Config) {
    c := cors.New(cors.Options{
        AllowedOrigins:   cfg.CORS.AllowedOrigins,
        AllowedMethods:   cfg.CORS.AllowedMethods,
        AllowedHeaders:   cfg.CORS.AllowedHeaders,
        AllowCredentials: true,
        MaxAge:           300,
        Debug:            false,
    })

    handler := c.Handler(r)
    
    srv := &http.Server{
        Handler:           handler,
        Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
        WriteTimeout:      cfg.Server.WriteTimeout,
        ReadTimeout:       cfg.Server.ReadTimeout,
        IdleTimeout:       cfg.Server.IdleTimeout,
        ReadHeaderTimeout: 10 * time.Second,
        MaxHeaderBytes:    1 << 20,
    }
    
    go func() {
        fmt.Printf("🚀 Server starting on %s:%s\n", cfg.Server.Host, cfg.Server.Port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server failed to start: %v", err)
        }
    }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    fmt.Println("🛑 Server shutting down...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }
    
    fmt.Println("✅ Server shutdown complete")
}