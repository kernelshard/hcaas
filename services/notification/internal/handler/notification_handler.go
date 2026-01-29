package handler

import (
	"encoding/json"
	"net/http"

	"github.com/kernelshard/hcaas/services/notification/internal/model"
	"github.com/kernelshard/hcaas/services/notification/internal/service"
	"github.com/samims/otelkit"
)

type NotificationHandler struct {
	service service.NotificationService
	tracer  *otelkit.Tracer
}

func NewNotificationHandler(s service.NotificationService, tracer *otelkit.Tracer) *NotificationHandler {
	return &NotificationHandler{service: s, tracer: tracer}
}

func (h *NotificationHandler) Notify(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), "NotificationHandler.Notify")
	defer span.End()

	var notification model.Notification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		otelkit.RecordError(span, err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	err := h.service.Send(ctx, &notification)
	if err != nil {
		otelkit.RecordError(span, err)
		http.Error(w, "failed to send notification", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Notification sent"))
}
