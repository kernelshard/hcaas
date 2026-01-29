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

	appErr "github.com/kernelshard/hcaas/services/auth/internal/errors"
	"github.com/kernelshard/hcaas/services/auth/internal/model"
	"github.com/kernelshard/hcaas/services/auth/internal/storage"
)

// AuthService defines the interface for authentication-related operations
type AuthService interface {
	Register(ctx context.Context, email, password string) (*model.User, error)
	Login(ctx context.Context, email, password string) (*model.User, string, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)
	ValidateToken(ctx context.Context, token string) (string, string, error)
}

// authService is the implementation of the AuthService interface
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

// NewAuthService creates a new instance of AuthService
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

// Register registers a new user
func (s *authService) Register(ctx context.Context, email, password string) (*model.User, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.Register")
	defer span.End()

	s.logger.Info("Register called", slog.String("email", email))
	span.SetAttributes(
		attribute.String("user.email", email),
		attribute.String("operation", "user_registration"),
	)

	if !regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`).MatchString(email) {
		s.logger.Error("Invalid email format", slog.String("email", email))
		span.SetStatus(codes.Error, "Invalid email format")
		span.SetAttributes(attribute.String("error.type", "invalid_email_format"))

		return nil, appErr.ErrInvalidEmail
	}

	if len(password) < 8 {
		s.logger.Error("Password too short", slog.Int("password_length", len(password)))
		span.SetStatus(codes.Error, "Password too short")
		span.SetAttributes(
			attribute.String("error.type", "password_too_short"),
			attribute.Int("password.length", len(password)),
		)
		return nil, appErr.ErrInvalidInput
	}

	// Enforce password complexity: at least one uppercase, one lowercase, one digit, one special char
	var (
		hasUpper   = regexp.MustCompile(`[A-Z]`).MatchString
		hasLower   = regexp.MustCompile(`[a-z]`).MatchString
		hasDigit   = regexp.MustCompile(`[0-9]`).MatchString
		hasSpecial = regexp.MustCompile(`[\W_]`).MatchString
	)

	complexityCheck := map[string]bool{
		"has_uppercase": hasUpper(password),
		"has_lowercase": hasLower(password),
		"has_digit":     hasDigit(password),
		"has_special":   hasSpecial(password),
	}

	// Check password complexity
	if !hasUpper(password) || !hasLower(password) || !hasDigit(password) || !hasSpecial(password) {
		s.logger.Error("Password does not meet complexity requirements", slog.Any("complexity_check", complexityCheck))
		span.SetStatus(codes.Error, "Password complexity requirements not met")
		span.SetAttributes(
			attribute.String("error.type", "password_complexity_failed"),
			attribute.Bool("complexity.has_uppercase", hasUpper(password)),
			attribute.Bool("complexity.has_lowercase", hasLower(password)),
			attribute.Bool("complexity.has_digit", hasDigit(password)),
			attribute.Bool("complexity.has_special", hasSpecial(password)),
		)
		return nil, appErr.ErrInvalidInput
	}

	// Hash the password
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("Password hashing failed", slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Password hashing failed")
		span.SetAttributes(attribute.String("error.type", "password_hashing_error"))
		return nil, appErr.ErrInternal
	}

	// Create the user from the provided email and hashed password
	createdUser, err := s.store.CreateUser(ctx, email, string(hashedPass))
	if err != nil {
		// Handle user creation errors
		// Check for conflict errors when user already exists
		if errors.Is(err, appErr.ErrConflict) {
			s.logger.Warn("User already exists", slog.String("email", email))
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, "User already exists")
			span.SetAttributes(attribute.String("error.type", "user_already_exists"))
			return nil, appErr.ErrConflict
		}

		// Log and record the error
		s.logger.Error("User creation failed", slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "User creation failed")
		span.SetAttributes(attribute.String("error.type", "user_creation_error"))

		return nil, appErr.ErrInternal
	}

	s.logger.Info("Register succeeded", slog.String("email", email), slog.String("user.id", createdUser.ID))
	span.SetAttributes(
		attribute.String("user.id", createdUser.ID),
		attribute.String("result", "success"),
	)
	span.AddEvent("user.registration.completed")
	span.AddEvent("operation.completed")

	return createdUser, nil
}

// Login logs in a user by email and password & generates a token
func (s *authService) Login(ctx context.Context, email, password string) (*model.User, string, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.Login")
	defer span.End()

	s.logger.Info("Login called", slog.String("email", email))
	span.SetAttributes(
		attribute.String("user.email", email),
		attribute.String("operation", "user_login"),
		attribute.String("service.component", "auth_service"),
	)

	// Check if user is locked out
	if lockoutUntil, locked := s.lockoutTime[email]; locked {
		if time.Now().Before(lockoutUntil) {
			remainingLockout := time.Until(lockoutUntil)

			s.logger.Warn("User account locked due to too many failed login attempts",
				slog.String("email", email),
				slog.Duration("remaining_lockout", remainingLockout))
			otelkit.RecordError(span, appErr.ErrTooManyAttempts)
			span.SetStatus(codes.Error, "Account locked due to too many failed attempts")
			span.SetAttributes(
				attribute.String("error.type", "account_locked"),
				attribute.Int64("lockout.remaining_seconds", int64(remainingLockout.Seconds())),
			)

			return nil, "", appErr.ErrTooManyAttempts
		} else {
			// Lockout period has expired, reset attempts
			delete(s.lockoutTime, email)
			s.loginAttempts[email] = 0
			span.AddEvent("lockout_expired_reset")
		}
	}

	// Fetch the user by email to validate credentials
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		// when there is no user found by email
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("User not found", slog.String("email", email))

			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, "User not found")
			span.SetAttributes(attribute.String("error.type", "user_not_found"))

			return nil, "", appErr.ErrUnauthorized
		}
		// for other database errors
		s.logger.Error("Failed to fetch user by email", slog.String("email", email), slog.Any("error", err))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Failed to fetch user by email")
		span.SetAttributes(attribute.String("error.type", "database_error"))

		return nil, "", appErr.ErrInternal
	}
	s.logger.Info("Log in user found", slog.String("email", email), slog.String("user.id", user.ID))
	span.SetAttributes(
		attribute.String("user.id", user.ID),
		attribute.String("user.found", "true"),
	)

	// Compare the provided password with the stored hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		// Password mismatch
		s.loginAttempts[email]++
		currentAttempts := s.loginAttempts[email]

		span.SetAttributes(
			attribute.Int("login.attempts", currentAttempts),
			attribute.Int("login.max_attempts", s.maxAttempts),
		)
		// Check if the user has reached the maximum number of login attempts
		if currentAttempts >= s.maxAttempts {
			s.lockoutTime[email] = time.Now().Add(s.lockoutDuration)
			s.logger.Warn("User account locked due to too many failed login attempts",
				slog.String("email", email),
				slog.Int("attempts", currentAttempts))
			span.SetStatus(codes.Error, "Account locked due to too many failed attempts")
			span.SetAttributes(
				attribute.String("error.type", "account_locked"),
				attribute.Int("login.failed_attempts", currentAttempts),
			)
			return nil, "", appErr.ErrTooManyAttempts
		}

		s.logger.Warn("Invalid password", slog.String("email", email), slog.Int("attempt", currentAttempts))

		span.SetStatus(codes.Error, "Invalid password")
		span.SetAttributes(
			attribute.String("error.type", "invalid_password"),
			attribute.Int("login.failed_attempts", currentAttempts),
		)

		return nil, "", appErr.ErrUnauthorized
	}

	// Reset login attempts on successful login
	s.loginAttempts[email] = 0
	span.AddEvent("login_attempts_reset")

	// Generate a token for the user
	token, err := s.tokenSvc.GenerateToken(ctx, user)
	if err != nil {
		// Token generation failed
		s.logger.Error("Token generation failed", slog.String("email", email), slog.Any("error", err))

		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Token generation failed")
		span.SetAttributes(attribute.String("error.type", "token_generation_error"))

		return nil, "", appErr.ErrTokenGeneration
	}

	s.logger.Info("Token Generated successfully", slog.String("email", email), slog.String("user.id", user.ID))
	span.SetAttributes(
		attribute.String("token.generated", "true"),
		attribute.String("result", "success"),
	)
	span.AddEvent("operation.completed")

	return user, token, nil
}

// GetUserByEmail gets a user by email and returns the user object
func (s *authService) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	ctx, span := s.tracer.StartServerSpan(ctx, "authService.GetUserByEmail")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.email", email),
		attribute.String("operation", "get_user_by_email"),
		attribute.String("service.component", "auth_service"),
	)

	// Fetch the user by email from the storage
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		// User not found
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Warn("User not found by email", slog.String("email", email))
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, "User not found by email")
			span.SetAttributes(attribute.String("error.type", "user_not_found"))

			return nil, appErr.ErrNotFound
		}
		// Failed to fetch user by email
		s.logger.Error(
			"Failed to fetch user by email",
			slog.String("email", email),
			slog.String("error", err.Error()),
		)

		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Failed to fetch user by email")
		span.SetAttributes(attribute.String("error.type", "database_error"))

		return nil, appErr.ErrInternal
	}

	s.logger.Info("User found by email", slog.String("email", email), slog.String("user.id", user.ID))
	span.SetAttributes(
		attribute.String("user.id", user.ID),
		attribute.String("result", "success"),
	)
	span.AddEvent("operation.completed")

	return user, nil
}

// ValidateToken validates a token and returns the user ID and email from the token
func (s *authService) ValidateToken(ctx context.Context, token string) (string, string, error) {
	_, span := s.tracer.StartServerSpan(ctx, "authService.ValidateToken")
	defer span.End()

	s.logger.Info("ValidateToken called")
	span.SetAttributes(
		attribute.String("token.present", "true"),
		attribute.String("operation", "validate_token"),
		attribute.String("service.component", "auth_service"),
	)

	// Validate the token
	userID, email, err := s.tokenSvc.ValidateToken(ctx, token)
	if err != nil {
		// Token validation failed
		s.logger.Info("Token validation failed", slog.String("error", err.Error()))
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, "Token validation failed")
		span.SetAttributes(attribute.String("error.type", "token_validation_error"))

		return "", "", err
	}

	// Token validation succeeded
	s.logger.Info("Token validation succeeded", slog.String("user.id", userID), slog.String("user.email", email))
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("user.email", email),
		attribute.String("result", "success"),
	)
	span.AddEvent("operation.completed")

	return userID, email, nil

}
