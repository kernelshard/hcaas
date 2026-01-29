package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kernelshard/hcaas/services/notification/internal/store"
)

// HealthService defines the interface for checking application health
type HealthService interface {
	Check(ctx context.Context) map[string]string
}

// healthService is the concrete implementation of the HealthService
type healthService struct {
	dbStorage store.NotificationStorage
}

// Check performs health checks on all critical dependencies
func (s healthService) Check(ctx context.Context) map[string]string {
	healthStatus := make(map[string]string)

	// check db conn
	// Use a timeout to prevent the health check from hanging
	dbCtx, dbCancel := context.WithTimeout(ctx, 2*time.Second)
	defer dbCancel()

	if err := s.dbStorage.Ping(dbCtx); err != nil {
		healthStatus["db"] = fmt.Sprintf("error: %s", err.Error())
	} else {
		healthStatus["db"] = "ok"
	}

	return healthStatus

}

// NewHealthService creates a new instance of the health check service
// It requires a database connection
// readiness checks on critical dependencies
func NewHealthService(dbStorage store.NotificationStorage) HealthService {
	return &healthService{dbStorage: dbStorage}
}
