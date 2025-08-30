/*
Health checks for the auth service
This service provides liveness and readiness checks for the authentication service.
*/
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/samims/hcaas/services/auth/internal/storage"
)

// HealthService defines the interface for health check operations
type HealthService interface {
	Liveness(ctx context.Context) error
	Readiness(ctx context.Context) error
}

// healthService is the implementation of the HealthService interface
type healthService struct {
	logger  *slog.Logger
	storage storage.UserStorage
}

// NewHealthService creates a new instance of HealthService
func NewHealthService(store storage.UserStorage, logger *slog.Logger) HealthService {
	l := logger.With("layer", "service", "component", "auth_health_service")
	return &healthService{storage: store, logger: l}

}

// Liveness checks if the service is alive
func (s *healthService) Liveness(ctx context.Context) error {
	s.logger.Debug("Liveness check passed")
	return nil
}

// Readiness checks if db service is ready
func (s *healthService) Readiness(ctx context.Context) error {
	s.logger.Debug("Readiness check initiated")
	// we wait upto 2 seconds
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := s.storage.Ping(ctx)
	if err != nil {
		s.logger.Error("Readiness check failed", slog.String("error", err.Error()))
		return err
	}

	s.logger.Debug("Readiness Check passed")
	return nil

}
