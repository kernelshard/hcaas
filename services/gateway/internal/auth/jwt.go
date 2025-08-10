// Package auth provides JWT authentication functionality for the API Gateway.
//
// This package implements JWT validation with proper error handling,
// context propagation, and metrics collection following Google's security practices.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator handles JWT token validation and user context extraction.
type JWTValidator struct {
	secretKey []byte
}

// NewJWTValidator creates a new JWT validator with the given secret key.
func NewJWTValidator(secretKey string) *JWTValidator {
	return &JWTValidator{
		secretKey: []byte(secretKey),
	}
}

// UserClaims represents the JWT claims structure.
// This should match the claims structure used by your auth service.
type UserClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// UserContext represents authenticated user context.
type UserContext struct {
	UserID    string
	Email     string
	TokenType string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// ValidateToken validates a JWT token and returns user context.
func (v *JWTValidator) ValidateToken(tokenString string) (*UserContext, error) {
	// Parse the token
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Validate token and extract claims
	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		// Additional validation
		if err := v.validateClaims(claims); err != nil {
			return nil, fmt.Errorf("invalid claims: %w", err)
		}

		return &UserContext{
			UserID:    claims.UserID,
			Email:     claims.Email,
			TokenType: "JWT",
			IssuedAt:  claims.IssuedAt.Time,
			ExpiresAt: claims.ExpiresAt.Time,
		}, nil
	}

	return nil, errors.New("invalid token")
}

// validateClaims performs additional validation on JWT claims.
func (v *JWTValidator) validateClaims(claims *UserClaims) error {
	now := time.Now()

	// Check if token is expired
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(now) {
		return errors.New("token is expired")
	}

	// Check if token is not valid yet (nbf - not before)
	if claims.NotBefore != nil && claims.NotBefore.After(now) {
		return errors.New("token is not valid yet")
	}

	// Validate required fields
	if claims.UserID == "" {
		return errors.New("missing user_id in token")
	}

	if claims.Email == "" {
		return errors.New("missing email in token")
	}

	return nil
}

// ExtractTokenFromHeader extracts JWT token from Authorization header.
// Expected format: "Bearer <token>"
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return "", errors.New("invalid authorization header format")
	}

	scheme := strings.ToLower(parts[0])
	if scheme != "bearer" {
		return "", errors.New("unsupported authorization scheme")
	}

	token := parts[1]
	if token == "" {
		return "", errors.New("missing token in authorization header")
	}

	return token, nil
}

// Context keys for storing user information
type contextKey string

const (
	UserContextKey contextKey = "user_context"
	UserIDKey      contextKey = "user_id"
)

// WithUserContext adds user context to the request context.
func WithUserContext(ctx context.Context, userCtx *UserContext) context.Context {
	ctx = context.WithValue(ctx, UserContextKey, userCtx)
	ctx = context.WithValue(ctx, UserIDKey, userCtx.UserID)
	return ctx
}

// GetUserContext extracts user context from request context.
func GetUserContext(ctx context.Context) (*UserContext, bool) {
	userCtx, ok := ctx.Value(UserContextKey).(*UserContext)
	return userCtx, ok
}

// GetUserID extracts user ID from request context.
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

// ValidationResult represents the result of token validation.
type ValidationResult string

const (
	ValidationSuccess ValidationResult = "success"
	ValidationExpired ValidationResult = "expired"
	ValidationInvalid ValidationResult = "invalid"
	ValidationMissing ValidationResult = "missing"
)

// GetValidationResult determines the validation result from an error.
func GetValidationResult(err error) ValidationResult {
	if err == nil {
		return ValidationSuccess
	}

	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "expired"):
		return ValidationExpired
	case strings.Contains(errStr, "missing"):
		return ValidationMissing
	default:
		return ValidationInvalid
	}
}

// TokenInfo provides additional information about a token for debugging/logging.
type TokenInfo struct {
	Valid     bool
	UserID    string
	Email     string
	IssuedAt  time.Time
	ExpiresAt time.Time
	TimeLeft  time.Duration
}

// GetTokenInfo extracts token information for logging/debugging purposes.
func (v *JWTValidator) GetTokenInfo(tokenString string) *TokenInfo {
	info := &TokenInfo{Valid: false}

	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		return v.secretKey, nil
	})

	if err != nil {
		return info
	}

	if claims, ok := token.Claims.(*UserClaims); ok {
		info.UserID = claims.UserID
		info.Email = claims.Email
		if claims.IssuedAt != nil {
			info.IssuedAt = claims.IssuedAt.Time
		}
		if claims.ExpiresAt != nil {
			info.ExpiresAt = claims.ExpiresAt.Time
			info.TimeLeft = time.Until(claims.ExpiresAt.Time)
		}
		info.Valid = token.Valid && info.TimeLeft > 0
	}

	return info
}
