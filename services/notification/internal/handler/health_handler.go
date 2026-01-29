package handler

import (
	"encoding/json"
	"net/http"

	"github.com/kernelshard/hcaas/services/notification/internal/service"
)

type HealthHandler struct {
	healthSvc service.HealthService
}

func NewHealthHandler(healthSvc service.HealthService) *HealthHandler {
	return &HealthHandler{healthSvc: healthSvc}
}

func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	data := h.healthSvc.Check(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if status, ok := data["db"]; ok && status == "ok" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(data)
}
