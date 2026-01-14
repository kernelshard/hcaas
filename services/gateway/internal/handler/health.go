package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/samims/hcaas/services/gateway/internal/config"
	"github.com/samims/hcaas/services/gateway/internal/logger"
)

// HealthHandler provides health check endpoints.
type HealthHandler struct {
	logger *logger.Logger
	config *config.ServicesConfig
	client *http.Client
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(logger *logger.Logger, cfg *config.ServicesConfig) *HealthHandler {
	return &HealthHandler{
		logger: logger.WithComponent("health-handler"),
		config: cfg,
		client: &http.Client{
			Timeout: 5 * time.Second, // Short timeout for health checks
		},
	}
}

// HealthResponse represents the structure of health check responses.
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Service   string            `json:"service"`
	Version   string            `json:"version,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// HealthCheck handles the /healthz endpoint.
func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	version := os.Getenv("SERVICE_VERSION")
	if version == "" {
		version = "dev"
	}

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Service:   "hcaas-gateway",
		Version:   version,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ReadinessCheck handles the /readyz endpoint with real upstream service validation.
func (h *HealthHandler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	checks := make(map[string]string)
	overallStatus := "ready"
	statusCode := http.StatusOK

	// Check Auth Service
	if h.checkUpstreamService(ctx, "auth", h.config.AuthService.BaseURL+"/healthz") {
		checks["auth_service"] = "healthy"
	} else {
		checks["auth_service"] = "unhealthy"
		overallStatus = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	// Check URL Service
	if h.checkUpstreamService(ctx, "url", h.config.URLService.BaseURL+"/health") {
		checks["url_service"] = "healthy"
	} else {
		checks["url_service"] = "unhealthy"
		overallStatus = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	// Check Notification Service
	if h.checkUpstreamService(ctx, "notification", h.config.NotificationService.BaseURL+"/healthz") {
		checks["notification_service"] = "healthy"
	} else {
		checks["notification_service"] = "unhealthy"
		overallStatus = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	// Internal checks
	checks["gateway_proxy"] = "ready"
	checks["middleware_stack"] = "ready"

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC(),
		Service:   "hcaas-gateway",
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// checkUpstreamService performs a health check on an upstream service.
func (h *HealthHandler) checkUpstreamService(ctx context.Context, serviceName, healthURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		h.logger.WithContext(ctx).Error("Failed to create health check request",
			logger.FieldService(serviceName),
			logger.FieldPath(healthURL),
		)
		return false
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.WithContext(ctx).Error("Health check request failed",
			logger.FieldService(serviceName),
			logger.FieldPath(healthURL),
		)
		return false
	}
	defer resp.Body.Close()

	// Consider 2xx status codes as healthy
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !healthy {
		h.logger.WithContext(ctx).Warn("Upstream service health check failed",
			logger.FieldService(serviceName),
			logger.FieldStatus(resp.StatusCode),
		)
	}

	return healthy
}
