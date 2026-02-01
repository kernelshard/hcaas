package middleware

import (
	"net/http"
	"time"

	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
)

// LoggingMiddleware provides request/response logging functionality.
type LoggingMiddleware struct {
	logger  *logger.Logger
	metrics *metrics.Metrics
}

// NewLoggingMiddleware creates a new logging middleware.
func NewLoggingMiddleware(logger *logger.Logger, metrics *metrics.Metrics) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger:  logger.WithComponent("logging-middleware"),
		metrics: metrics,
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(data)
}

// Middleware returns the logging middleware handler.
func (m *LoggingMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			
			// Wrap the response writer
			wrappedWriter := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}
			
			// Process the request
			next.ServeHTTP(wrappedWriter, r)
			
			// Calculate duration
			duration := time.Since(startTime)
			
			// Log the request
			m.logger.LogRequest(
				r.Context(),
				r.Method,
				r.URL.Path,
				r.UserAgent(),
				wrappedWriter.statusCode,
				int(duration.Milliseconds()),
			)
			
			// Record metrics
			m.metrics.RecordHTTPRequest(
				r.Method,
				r.URL.Path,
				wrappedWriter.statusCode,
				duration,
			)
		})
	}
}
