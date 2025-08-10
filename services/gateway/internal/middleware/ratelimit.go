package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
)

// RateLimitMiddleware provides rate limiting functionality using token bucket algorithm.
type RateLimitMiddleware struct {
	limiters map[string]*rate.Limiter
	mutex    sync.RWMutex
	
	requestsPerSecond int
	burstSize         int
	
	logger  *logger.Logger
	metrics *metrics.Metrics
	
	// Last access time tracking for cleanup
	lastAccess map[string]time.Time
	
	// Cleanup ticker for removing old limiters
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewRateLimitMiddleware creates a new rate limiting middleware.
func NewRateLimitMiddleware(requestsPerSecond, burstSize int, logger *logger.Logger, metrics *metrics.Metrics) *RateLimitMiddleware {
	rl := &RateLimitMiddleware{
		limiters:          make(map[string]*rate.Limiter),
		lastAccess:        make(map[string]time.Time),
		requestsPerSecond: requestsPerSecond,
		burstSize:         burstSize,
		logger:            logger.WithComponent("rate-limit"),
		metrics:           metrics,
		stopCleanup:       make(chan struct{}),
	}
	
	// Start cleanup goroutine to remove inactive limiters
	rl.startCleanup()
	
	return rl
}

// Middleware returns the rate limiting middleware handler.
func (rl *RateLimitMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client identifier (IP address)
			clientIP := rl.getClientIP(r)
			
			// Get or create limiter for this client
			limiter := rl.getLimiter(clientIP)
			
			// Check if request is allowed
			if !limiter.Allow() {
				rl.handleRateLimitExceeded(w, r, clientIP)
				return
			}
			
			// Request is allowed, proceed
			next.ServeHTTP(w, r)
		})
	}
}

// getLimiter gets or creates a rate limiter for a client.
func (rl *RateLimitMiddleware) getLimiter(clientIP string) *rate.Limiter {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	limiter, exists := rl.limiters[clientIP]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(rl.requestsPerSecond), rl.burstSize)
		rl.limiters[clientIP] = limiter
	}
	
	// Update last access time for cleanup
	rl.lastAccess[clientIP] = time.Now()
	
	return limiter
}

// getLimiterForIP gets an existing limiter for rate limit header calculation.
func (rl *RateLimitMiddleware) getLimiterForIP(clientIP string) *rate.Limiter {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()
	
	if limiter, exists := rl.limiters[clientIP]; exists {
		return limiter
	}
	
	// Return a default limiter if not found
	return rate.NewLimiter(rate.Limit(rl.requestsPerSecond), rl.burstSize)
}

// handleRateLimitExceeded handles rate limit exceeded scenarios.
func (rl *RateLimitMiddleware) handleRateLimitExceeded(w http.ResponseWriter, r *http.Request, clientIP string) {
	// Record rate limit hit
	rl.metrics.RecordRateLimitHit(clientIP, r.URL.Path)
	
	// Log the rate limit hit
	rl.logger.WithContext(r.Context()).Warn("Rate limit exceeded",
		logger.FieldPath(r.URL.Path),
		logger.FieldMethod(r.Method),
		logger.FieldStatus(http.StatusTooManyRequests),
		logger.FieldUserID(clientIP),
	)
	
	// Calculate remaining tokens and reset time
	limiter := rl.getLimiterForIP(clientIP)
	remaining := int(limiter.Tokens())
	resetTime := 60 // Approximate reset time based on rate
	
	// Set rate limit headers with actual values
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.requestsPerSecond))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetTime))
	w.Header().Set("Retry-After", strconv.Itoa(resetTime))
	
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	
	// Set content type and status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	
	// Write error response
	errorResponse := `{"error": "Rate limit exceeded", "code": "RATE_LIMIT_EXCEEDED", "retry_after": 60}`
	w.Write([]byte(errorResponse))
}

// getClientIP extracts client IP from request.
func (rl *RateLimitMiddleware) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// startCleanup starts a goroutine to periodically clean up inactive limiters.
func (rl *RateLimitMiddleware) startCleanup() {
	rl.cleanupTicker = time.NewTicker(5 * time.Minute) // Clean up every 5 minutes
	
	go func() {
		for {
			select {
			case <-rl.cleanupTicker.C:
				rl.cleanup()
			case <-rl.stopCleanup:
				rl.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanup removes limiters that haven't been used recently.
func (rl *RateLimitMiddleware) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	cleanupThreshold := 10 * time.Minute
	cleanupCount := 0
	
	for clientIP, lastAccess := range rl.lastAccess {
		// Remove limiters that haven't been accessed in the last 10 minutes
		if now.Sub(lastAccess) > cleanupThreshold {
			delete(rl.limiters, clientIP)
			delete(rl.lastAccess, clientIP)
			cleanupCount++
		}
	}
	
	// Update metrics with active limiter count
	rl.metrics.SetRateLimitActive("*", float64(len(rl.limiters)))
	
	rl.logger.Debug("Rate limiter cleanup completed",
		logger.FieldService("rate-limiter"),
		slog.Int("cleaned_up", cleanupCount),
		slog.Int("active_limiters", len(rl.limiters)),
	)
}

// Stop stops the rate limiter cleanup goroutine.
func (rl *RateLimitMiddleware) Stop() {
	close(rl.stopCleanup)
}

// GetStats returns current rate limiting statistics.
func (rl *RateLimitMiddleware) GetStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()
	
	return map[string]interface{}{
		"active_limiters":     len(rl.limiters),
		"requests_per_second": rl.requestsPerSecond,
		"burst_size":          rl.burstSize,
	}
}
