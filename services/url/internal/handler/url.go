package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"

	"github.com/kernelshard/hcaas/services/url/internal/errors"
	"github.com/kernelshard/hcaas/services/url/internal/model"
	"github.com/kernelshard/hcaas/services/url/internal/service"
	"github.com/samims/otelkit"
)

type URLHandler struct {
	svc    service.URLService
	logger *slog.Logger
	tracer *otelkit.Tracer
}

// Static string for the span names to avoid magic string
const (
	spanGetAll         = "auth.handler.GetAll"
	spanGetAllByUserID = "auth.handler.GetAllByUserID"
	spanGetByID        = "auth.handler.GetByID"
	spanAdd            = "auth.handler.Add"
	spanUpdateStatus   = "auth.handler.UpdateStatus"
)

func NewURLHandler(s service.URLService, logger *slog.Logger, tracer *otelkit.Tracer) *URLHandler {
	return &URLHandler{svc: s, logger: logger, tracer: tracer}
}

func (h *URLHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), spanGetAll)
	defer span.End()

	urls, err := h.svc.GetAll(ctx)
	if err != nil {
		otelkit.RecordError(span, err)
		h.logger.Error("GetAll failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(urls)
}

func (h *URLHandler) GetAllByUserID(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), spanGetAllByUserID)
	defer span.End()

	urls, err := h.svc.GetAllByUserID(ctx)
	if err != nil {
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Error("GetAllByUserID failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(urls)
}

func (h *URLHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), spanGetByID)
	defer span.End()

	id := chi.URLParam(r, "id")
	url, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.IsNotFound(err) {
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			h.logger.Warn("URL not found", "id", id)
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			h.logger.Error("GetByID failed", "id", id, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(url)
}

func (h *URLHandler) Add(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), spanAdd)
	defer span.End()

	var url model.URL
	if err := json.NewDecoder(r.Body).Decode(&url); err != nil {
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())
		h.logger.Warn("Invalid request body for Add")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	url.Status = model.StatusUnknown

	if err := h.svc.Add(ctx, url); err != nil {
		if errors.IsInternal(err) {
			h.logger.Warn("Duplicate or invalid Add", "url", url, "error", err)
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			h.logger.Error("Add failed", "url", url, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *URLHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), spanUpdateStatus)
	defer span.End()

	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.logger.Warn("Invalid request body for UpdateStatus", "id", id)
		otelkit.RecordError(span, err)
		span.SetStatus(codes.Error, err.Error())

		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateStatus(ctx, id, body.Status); err != nil {
		if errors.IsNotFound(err) {
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())

			h.logger.Warn("URL not found for update", "id", id)
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			otelkit.RecordError(span, err)
			span.SetStatus(codes.Error, err.Error())
			h.logger.Error("UpdateStatus failed", "id", id, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
}
