package service

import (
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/samims/hcaas/services/auth/internal/model"
)

type TokenService interface {
	GenerateToken(user *model.User) (string, error)
	ValidateToken(tokenStr string) (string, string, error)
}

type jwtService struct {
	secret     string
	expiryTime time.Duration
	logger     *slog.Logger
}

func NewJWTService(secret string, expiry time.Duration, logger *slog.Logger) TokenService {
	return &jwtService{secret: secret, expiryTime: expiry, logger: logger}
}

func (s *jwtService) GenerateToken(user *model.User) (string, error) {
	if user == nil {
		s.logger.Error("error generating token")
		return "", errors.New("user is nil: cannot generate token")
	}
	s.logger.Info("token expiry time", slog.Duration("time", s.expiryTime))

	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(s.expiryTime).Unix(),
		"iat":   time.Now().Unix(),
		"nbf":   time.Now().Unix(), // Not valid before now
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

func (s *jwtService) ValidateToken(tokenStr string) (string, string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		// Validate the signing method to prevent algorithm confuses attack
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			s.logger.Error("Unexpected signing method", slog.String("method", token.Header["alg"].(string)))
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.secret), nil
	})

	if err != nil || !token.Valid {
		s.logger.Error("Invalid token ")
		return "", "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		s.logger.Error("token verification failed malformed")
		return "", "", jwt.ErrTokenMalformed
	}
	userID, ok := claims["sub"].(string)
	if !ok {
		s.logger.Error("token verification failed malformed!")
		return "", "", jwt.ErrTokenMalformed
	}
	email, ok := claims["email"].(string)
	if !ok {
		s.logger.Error("Invalid email claim", slog.String("email", email))
		return "", "", jwt.ErrTokenMalformed
	}

	// validate time based claims
	now := time.Now().Unix()

	// check expiry
	if exp, ok := claims["exp"].(float64); ok {
		if int64(exp) < now {
			s.logger.Error("Token expired", slog.Int64("exp", int64(exp)), slog.Int64("now", now))
			return "", "", jwt.ErrTokenExpired
		}
	}

	// check not before
	if nbf, ok := claims["nbf"].(float64); ok {
		if int64(nbf) > now {
			s.logger.Error("Token not valid yet", slog.Int64("nbf", int64(nbf)), slog.Int64("now", now))
			return "", "", jwt.ErrTokenNotValidYet
		}
	}

	return userID, email, nil
}
