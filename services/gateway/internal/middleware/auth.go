// Package middleware provides HTTP middleware for the API Gateway.
//
// This package implements authentication, authorization, and request processing
// middleware following Google's security and observability best practices.
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/samims/hcaas/services/gateway/internal/auth"
	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
)

// AuthMiddleware provides JWT authentication middleware.
type AuthMiddleware struct {
	validator *auth.JWTValidator
	logger    *logger.Logger
	metrics   *metrics.Metrics
	
	// Paths that don't require authentication
	publicPaths []string
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(jwtSecret string, logger *logger.Logger, metrics *metrics.Metrics) *AuthMiddleware {
	publicPaths := []string{
		"/auth/register",
		"/auth/login",
		"/auth/validate",
		"/healthz",
		"/health",
		"/readyz",
		"/metrics",
	}

	return &AuthMiddleware{
		validator:   auth.NewJWTValidator(jwtSecret),
		logger:      logger.WithComponent("auth-middleware"),
		metrics:     metrics,
		publicPaths: publicPaths,
	}
}

// Middleware returns the authentication middleware handler.
func (m *AuthMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			
			// Skip authentication for public paths
			if m.isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			token, err := auth.ExtractTokenFromHeader(authHeader)
			
			if err != nil {
				m.handleAuthError(w, r, err, auth.ValidationMissing)
				m.recordAuthMetrics(auth.ValidationMissing, time.Since(startTime))
				return
			}

			// Validate the token
			userCtx, err := m.validator.ValidateToken(token)
			if err != nil {
				validationResult := auth.GetValidationResult(err)
				m.handleAuthError(w, r, err, validationResult)
				m.recordAuthMetrics(validationResult, time.Since(startTime))
				return
			}

			// Add user context to request
			ctx := auth.WithUserContext(r.Context(), userCtx)
			r = r.WithContext(ctx)

			// Record successful authentication
			m.recordAuthMetrics(auth.ValidationSuccess, time.Since(startTime))
			
			// Log successful authentication
			m.logger.WithContext(ctx).Info("Authentication successful",
				logger.FieldUserID(userCtx.UserID),
				logger.FieldPath(r.URL.Path),
				logger.FieldMethod(r.Method),
			)

			next.ServeHTTP(w, r)
		})
	}
}

// isPublicPath checks if a path doesn't require authentication.
func (m *AuthMiddleware) isPublicPath(path string) bool {
	for _, publicPath := range m.publicPaths {
		if strings.HasPrefix(path, publicPath) {
			return true
		}
	}
	return false
}

// handleAuthError handles authentication errors with appropriate HTTP responses.
func (m *AuthMiddleware) handleAuthError(w http.ResponseWriter, r *http.Request, err error, result auth.ValidationResult) {
	m.logger.WithContext(r.Context()).Info("Authentication failed",
		logger.FieldPath(r.URL.Path),
		logger.FieldMethod(r.Method),
		logger.FieldStatus(http.StatusUnauthorized),
	)

	// Set appropriate status code based on error type
	statusCode := http.StatusUnauthorized
	message := "Authentication required"

	switch result {
	case auth.ValidationExpired:
		message = "Token has expired"
	case auth.ValidationInvalid:
		message = "Invalid token"
	case auth.ValidationMissing:
		message = "Authorization header required"
	}

	// Set JSON content type and CORS headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

	w.WriteHeader(statusCode)
	
	// Write JSON error response
	errorResponse := `{"error": "` + message + `", "code": "AUTHENTICATION_FAILED"}`
	w.Write([]byte(errorResponse))
}

// recordAuthMetrics records authentication metrics.
func (m *AuthMiddleware) recordAuthMetrics(result auth.ValidationResult, duration time.Duration) {
	m.metrics.RecordAuthValidation(string(result), duration)
}

// OptionalAuthMiddleware provides optional authentication middleware.
// This allows authenticated and unauthenticated requests to proceed, but adds
// user context if authentication is present and valid.
type OptionalAuthMiddleware struct {
	validator *auth.JWTValidator
	logger    *logger.Logger
	metrics   *metrics.Metrics
}

// NewOptionalAuthMiddleware creates a new optional authentication middleware.
func NewOptionalAuthMiddleware(jwtSecret string, logger *logger.Logger, metrics *metrics.Metrics) *OptionalAuthMiddleware {
	return &OptionalAuthMiddleware{
		validator: auth.NewJWTValidator(jwtSecret),
		logger:    logger.WithComponent("optional-auth"),
		metrics:   metrics,
	}
}

// Middleware returns the optional authentication middleware handler.
func (m *OptionalAuthMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			
			// Try to extract and validate token
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				if token, err := auth.ExtractTokenFromHeader(authHeader); err == nil {
					if userCtx, err := m.validator.ValidateToken(token); err == nil {
						// Add user context to request
						ctx := auth.WithUserContext(r.Context(), userCtx)
						r = r.WithContext(ctx)
						
						m.recordAuthMetrics(auth.ValidationSuccess, time.Since(startTime))
					} else {
						// Log validation failure but don't block request
						validationResult := auth.GetValidationResult(err)
						m.recordAuthMetrics(validationResult, time.Since(startTime))
						
						m.logger.WithContext(r.Context()).Debug("Optional auth validation failed",
							logger.FieldPath(r.URL.Path),
							logger.FieldMethod(r.Method),
						)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// recordAuthMetrics records authentication metrics for optional auth.
func (m *OptionalAuthMiddleware) recordAuthMetrics(result auth.ValidationResult, duration time.Duration) {
	m.metrics.RecordAuthValidation(string(result), duration)
}
