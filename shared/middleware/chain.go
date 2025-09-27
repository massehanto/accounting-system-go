// shared/middleware/chain.go
package middleware

import "net/http"

type Middleware func(http.HandlerFunc) http.HandlerFunc

// Chain applies middlewares to a handler in reverse order
func Chain(middlewares ...Middleware) func(http.HandlerFunc) http.HandlerFunc {
    return func(final http.HandlerFunc) http.HandlerFunc {
        for i := len(middlewares) - 1; i >= 0; i-- {
            final = middlewares[i](final)
        }
        return final
    }
}

// Common middleware combinations
func APIMiddleware(jwtSecret string) func(http.HandlerFunc) http.HandlerFunc {
    return Chain(
        SecurityHeaders,
        RateLimit(60),
        LoggingMiddleware,
        NewAuthMiddleware(jwtSecret),
    )
}

func PublicMiddleware() func(http.HandlerFunc) http.HandlerFunc {
    return Chain(
        SecurityHeaders,
        RateLimit(20),
        LoggingMiddleware,
    )
}