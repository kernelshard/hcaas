package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/samims/hcaas/services/auth/internal/model"
	"github.com/samims/otelkit"
)

// TokenService defines the interface for token-related operations
type TokenService interface {
	GenerateToken(ctx context.Context, user *model.User) (string, error)
	ValidateToken(ctx context.Context, tokenStr string) (string, string, error)
}

// jwtService is the implementation of the TokenService interface
type jwtService struct {
	secret     string
	expiryTime time.Duration
	logger     *slog.Logger
	tracer     *otelkit.Tracer
}

// NewJWTService creates a new instance of TokenService
func NewJWTService(secret string, expiry time.Duration, logger *slog.Logger, tracer *otelkit.Tracer) TokenService {
	return &jwtService{secret: secret, expiryTime: expiry, logger: logger, tracer: tracer}
}

// GenerateToken generates a new JWT token for a user
func (s *jwtService) GenerateToken(ctx context.Context, user *model.User) (string, error) {
	// Start a new span for tracing
	ctx, span := s.tracer.Start(ctx, "auth.service.GenerateToken")
	defer span.End()

	if user == nil {
		err := errors.New("user is nil: cannot generate token")
		s.logger.Error("error generating token")
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())

		return "", err
	}
	s.logger.Info("token expiry time", slog.Duration("time", s.expiryTime))

	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(s.expiryTime).Unix(),
		"iat":   time.Now().Unix(),
		"nbf":   time.Now().Unix(), // Not valid before now
	}
	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

// ValidateToken validates a JWT token and returns the user ID and email if valid
func (s *jwtService) ValidateToken(ctx context.Context, tokenStr string) (string, string, error) {

	ctx, span := s.tracer.Start(ctx, "auth.service.ValidateToken")
	defer span.End()

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		// Validate the signing method to prevent algorithm confuses attack
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			s.logger.Error("Unexpected signing method", slog.String("method", token.Header["alg"].(string)))
			otelkit.RecordError(span, jwt.ErrSignatureInvalid)
			span.SetStatus(codes.Error, jwt.ErrSignatureInvalid.Error())

			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.secret), nil
	})

	// Check if the token is valid
	if err != nil || !token.Valid {
		s.logger.Error("Invalid token ")

		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())

		return "", "", err
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		s.logger.Error("token verification failed malformed")
		otelkit.RecordError(span, jwt.ErrTokenMalformed)
		span.SetStatus(codes.Error, jwt.ErrTokenMalformed.Error())

		return "", "", jwt.ErrTokenMalformed
	}

	userID, ok := claims["sub"].(string)

	if !ok {
		s.logger.Error("token verification failed malformed!")
		otelkit.RecordError(span, jwt.ErrTokenMalformed)
		span.SetStatus(codes.Error, jwt.ErrTokenMalformed.Error())

		return "", "", jwt.ErrTokenMalformed
	}

	email, ok := claims["email"].(string)

	if !ok {
		otelkit.RecordError(span, jwt.ErrTokenMalformed)
		span.SetStatus(codes.Error, jwt.ErrTokenMalformed.Error())
		s.logger.Error("Invalid email claim", slog.String("email", email))
		return "", "", jwt.ErrTokenMalformed
	}

	// validate time based claims
	now := time.Now().Unix()

	// check expiry
	if exp, ok := claims["exp"].(float64); ok {
		if int64(exp) < now {
			s.logger.Error("Token expired", slog.Int64("exp", int64(exp)), slog.Int64("now", now))
			otelkit.RecordError(span, jwt.ErrTokenExpired)
			span.SetStatus(codes.Error, jwt.ErrTokenExpired.Error())
			return "", "", jwt.ErrTokenExpired
		}
	}

	// check not before
	if nbf, ok := claims["nbf"].(float64); ok {
		if int64(nbf) > now {
			s.logger.Error("Token not valid yet", slog.Int64("nbf", int64(nbf)), slog.Int64("now", now))
			otelkit.RecordError(span, jwt.ErrTokenNotValidYet)
			span.SetStatus(codes.Error, jwt.ErrTokenNotValidYet.Error())
			return "", "", jwt.ErrTokenNotValidYet
		}
	}

	// add event
	span.AddEvent("Token validated", trace.WithAttributes(
		attribute.String("user_id", userID),
		attribute.String("email", email),
	))
	return userID, email, nil
}
