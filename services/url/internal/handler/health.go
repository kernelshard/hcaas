package handler

import (
	"log/slog"
	"net/http"

	"github.com/kernelshard/hcaas/services/url/internal/service"
)

type HealthHandler struct {
	service service.HealthService
	logger  *slog.Logger
}

func NewHealthHandler(svc service.HealthService, l *slog.Logger) *HealthHandler {
	return &HealthHandler{service: svc, logger: l}
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	err := h.service.Liveness(r.Context())
	if err != nil {
		http.Error(w, "unhealthy", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	err := h.service.Readiness(r.Context())
	if err != nil {
		http.Error(w, "not ready", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))

}
