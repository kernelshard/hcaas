package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"

	"github.com/jackc/pgx/v5"
	"github.com/samims/otelkit"

	appErr "github.com/samims/hcaas/services/auth/internal/errors"
	"github.com/samims/hcaas/services/auth/internal/model"
	"github.com/samims/hcaas/services/auth/internal/storage"
)

type AuthService interface {
	Register(ctx context.Context, email, password string) (*model.User, error)
	Login(ctx context.Context, email, password string) (*model.User, string, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	ValidateToken(token string) (string, string, error)
}

type authService struct {
	store    storage.UserStorage
	logger   *slog.Logger
	tokenSvc TokenService
	tracer   *otelkit.Tracer
	// Add rate limiting map or store here if needed
	loginAttempts   map[string]int
	lockoutTime     map[string]time.Time
	lockoutDuration time.Duration
	maxAttempts     int
}

func NewAuthService(store storage.UserStorage, logger *slog.Logger, tokenSvc TokenService, tracer *otelkit.Tracer) AuthService {
	l := logger.With("layer", "service", "component", "authService")
	return &authService{
		store:           store,
		logger:          l,
		tokenSvc:        tokenSvc,
		tracer:          tracer,
		loginAttempts:   make(map[string]int),
		lockoutTime:     make(map[string]time.Time),
		lockoutDuration: 15 * time.Minute,
		maxAttempts:     5,
	}
}

func (s *authService) Register(ctx context.Context, email, password string) (*model.User, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.Register")
	defer span.End()

	s.logger.Info("Register called", slog.String("email", email))
	span.SetAttributes(attribute.String("user.email", email))

	if !regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`).MatchString(email) {
		s.logger.Error("Invalid email")
		span.SetStatus(codes.Error, "Invalid email format")
		return nil, appErr.ErrInvalidEmail
	}

	if len(password) < 8 {
		s.logger.Error("Password too short")
		span.SetStatus(codes.Error, "Password too short")
		return nil, appErr.ErrInvalidInput
	}

	// Enforce password complexity: at least one uppercase, one lowercase, one digit, one special char
	var (
		hasUpper   = regexp.MustCompile(`[A-Z]`).MatchString
		hasLower   = regexp.MustCompile(`[a-z]`).MatchString
		hasDigit   = regexp.MustCompile(`[0-9]`).MatchString
		hasSpecial = regexp.MustCompile(`[\W_]`).MatchString
	)
	if !hasUpper(password) || !hasLower(password) || !hasDigit(password) || !hasSpecial(password) {
		s.logger.Error("Password does not meet complexity requirements")
		span.SetStatus(codes.Error, "Password complexity requirements not met")
		return nil, appErr.ErrInvalidInput
	}

	hashedPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Password hashing failed", slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Password hashing failed")
		return nil, appErr.ErrInternal
	}

	createdUser, err := s.store.CreateUser(ctx, email, string(hashedPass))
	if err != nil {
		if errors.Is(err, appErr.ErrConflict) {
			s.logger.Warn("User already exists", slog.String("email", email))
			span.SetStatus(codes.Error, "User already exists")
			return nil, appErr.ErrConflict
		}
		s.logger.Error("User creation failed", slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "User creation failed")
		return nil, appErr.ErrInternal
	}

	s.logger.Info("Register succeeded", slog.String("email", email))
	span.SetAttributes(attribute.String("user.id", createdUser.ID))
	return createdUser, nil
}

func (s *authService) Login(ctx context.Context, email, password string) (*model.User, string, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.Login")
	defer span.End()

	s.logger.Info("Login called", slog.String("email", email))
	span.SetAttributes(attribute.String("user.email", email))

	// Check if user is locked out
	if lockoutUntil, locked := s.lockoutTime[email]; locked {
		if time.Now().Before(lockoutUntil) {
			s.logger.Warn("User account locked due to too many failed login attempts", slog.String("email", email))
			otelkit.RecordError(span, errors.New("too many attempts"))
			span.SetStatus(codes.Error, "Account locked due to too many failed attempts")
			return nil, "", appErr.ErrTooManyAttempts
		} else {
			// Lockout expired, reset
			delete(s.lockoutTime, email)
			s.loginAttempts[email] = 0
		}
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("User not found", slog.String("email", email))
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, "User not found")
			return nil, "", appErr.ErrUnauthorized
		}
		s.logger.Error("Failed to fetch user by email", slog.String("email", email), slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Failed to fetch user by email")
		return nil, "", appErr.ErrInternal
	}
	s.logger.Info("Log in user found", slog.String("email", email))
	span.SetAttributes(attribute.String("user.id", user.ID))

	// Compare the provided password with the stored hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.loginAttempts[email]++
		if s.loginAttempts[email] >= s.maxAttempts {
			s.lockoutTime[email] = time.Now().Add(s.lockoutDuration)
			s.logger.Warn("User account locked due to too many failed login attempts", slog.String("email", email))
			span.SetStatus(codes.Error, "Account locked due to too many failed attempts")
			return nil, "", appErr.ErrTooManyAttempts
		}
		s.logger.Warn("Invalid password", slog.String("email", email))
		span.SetStatus(codes.Error, "Invalid password")
		return nil, "", appErr.ErrUnauthorized
	}

	// Reset login attempts on successful login
	s.loginAttempts[email] = 0

	token, err := s.tokenSvc.GenerateToken(user)
	if err != nil {
		s.logger.Error("Token generation failed ", slog.String("email", email))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Token generation failed")
		return nil, "", appErr.ErrTokenGeneration
	}

	s.logger.Info("Token Generated successfully", slog.String("email", email))
	span.SetAttributes(attribute.String("token.generated", "true"))
	return user, token, nil
}

func (s *authService) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.GetUserByEmail")
	defer span.End()

	span.SetAttributes(attribute.String("user.email", email))

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		s.logger.Error(
			"Failed to fetch user by email ",
			slog.String("email", email),
			slog.String("error", err.Error()),
		)
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Failed to fetch user by email")
		return nil, appErr.ErrInternal
	}

	span.SetAttributes(attribute.String("user.id", user.ID))
	return user, nil
}

func (s *authService) ValidateToken(token string) (string, string, error) {
	_, span := s.tracer.StartServerSpan(context.Background(), "authService.ValidateToken")
	defer span.End()

	s.logger.Info("ValidateToken called")
	span.SetAttributes(attribute.String("token.present", "true"))

	userID, email, err := s.tokenSvc.ValidateToken(token)
	if err != nil {
		s.logger.Info("Token validation failed", slog.String("error", err.Error()))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Token validation failed")
		return "", "", err
	}

	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("user.email", email),
	)
	return userID, email, nil

}
