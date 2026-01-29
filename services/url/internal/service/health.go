package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/kernelshard/hcaas/services/url/internal/storage"
)

type HealthService interface {
	Liveness(ctx context.Context) error
	Readiness(ctx context.Context) error
}

type healthService struct {
	store  storage.Storage
	logger *slog.Logger
}

func (s healthService) Liveness(ctx context.Context) error {
	s.logger.Debug("Liveness check passed")
	return nil
}

func (s healthService) Readiness(ctx context.Context) error {
	s.logger.Debug("Readiness check initiated")
	// we wait upto 2 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := s.store.Ping(ctx)
	if err != nil {
		s.logger.Error("Readiness check failed", slog.String("error", err.Error()))
		return err
	}
	s.logger.Debug("Readiness Check passed")
	return nil
}

func NewHealthService(store storage.Storage, logger *slog.Logger) HealthService {
	l := logger.With("layer", "service", "component", "healthService")
	return &healthService{store: store, logger: l}
}
