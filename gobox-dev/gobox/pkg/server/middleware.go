package server

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"go.uber.org/zap"
)

// loggingMiddleware logs HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request
		s.logger.Info("HTTP request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", wrapped.statusCode),
			zap.Duration("duration", time.Since(start)),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
	})
}

// authMiddleware validates authentication tokens.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks
		if r.URL.Path == "/" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing authorization header")
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid authorization header format")
			return
		}

		token := parts[1]
		if token != s.config.AuthToken {
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		// Check if origin is allowed
		allowed := false
		for _, o := range s.config.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware recovers from panics.
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("Panic recovered",
					zap.Any("error", err),
					zap.String("stack", string(debug.Stack())),
				)
				s.sendError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// timeoutMiddleware adds request timeout.
func (s *Server) timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddleware limits request rate.
// Note: This is a simple implementation. For production, use a proper rate limiter.
type rateLimiter struct {
	tokens    chan struct{}
	rps       int
	burstSize int
}

func newRateLimiter(rps, burstSize int) *rateLimiter {
	rl := &rateLimiter{
		tokens:    make(chan struct{}, burstSize),
		rps:       rps,
		burstSize: burstSize,
	}

	// Fill initial tokens
	for i := 0; i < burstSize; i++ {
		rl.tokens <- struct{}{}
	}

	// Refill tokens periodically
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()

		for range ticker.C {
			select {
			case rl.tokens <- struct{}{}:
			default:
			}
		}
	}()

	return rl
}

func (rl *rateLimiter) allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := newRateLimiter(s.config.RateLimitRPS, s.config.RateLimitBurst)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow() {
			s.sendError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestID middleware adds a unique request ID.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for existing request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Add to response headers
		w.Header().Set("X-Request-ID", requestID)

		// Add to request context
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey string

const requestIDKey contextKey = "request_id"

func generateRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
