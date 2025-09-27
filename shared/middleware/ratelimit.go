// shared/middleware/ratelimit.go - CORRECTED VERSION
package middleware

import (
    "fmt"
    "net/http"
    "sync"
    "time"
    
    "golang.org/x/time/rate"
)

type MemoryRateLimiter struct {
    mu       sync.RWMutex
    limiters map[string]*rate.Limiter
    cleanup  time.Time
}

func NewMemoryRateLimiter() *MemoryRateLimiter {
    return &MemoryRateLimiter{
        limiters: make(map[string]*rate.Limiter),
        cleanup:  time.Now(),
    }
}

var globalLimiter = NewMemoryRateLimiter()

func RateLimit(requestsPerMinute int) func(http.HandlerFunc) http.HandlerFunc {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            key := getClientIP(r)
            
            if !globalLimiter.Allow(key, requestsPerMinute) {
                w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", requestsPerMinute))
                w.Header().Set("X-RateLimit-Remaining", "0")
                w.Header().Set("Retry-After", "60")
                respondWithError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", 
                    "Too many requests. Please try again later.")
                return
            }
            
            next(w, r)
        }
    }
}

func (rl *MemoryRateLimiter) Allow(key string, requestsPerMinute int) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    // Clean up old limiters every 5 minutes
    if time.Since(rl.cleanup) > 5*time.Minute {
        rl.limiters = make(map[string]*rate.Limiter)
        rl.cleanup = time.Now()
    }
    
    limiter, exists := rl.limiters[key]
    if !exists {
        limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), requestsPerMinute)
        rl.limiters[key] = limiter
    }
    
    return limiter.Allow()
}