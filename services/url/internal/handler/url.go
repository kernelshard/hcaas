package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/url/internal/errors"
	"github.com/samims/hcaas/services/url/internal/model"
	"github.com/samims/hcaas/services/url/internal/service"
)

type URLHandler struct {
	svc    service.URLService
	logger *slog.Logger
}

func NewURLHandler(s service.URLService, logger *slog.Logger) *URLHandler {
	return &URLHandler{svc: s, logger: logger}
}

func (h *URLHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	tracer := tracing.NewTracer(tracing.GetTracer("url-handler"))
	ctx, span := tracer.StartServerSpan(r.Context(), "GetAll")
	defer span.End()

	urls, err := h.svc.GetAll(ctx)
	if err != nil {
		tracer.RecordError(span, err)
		h.logger.Error("GetAll failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(urls)
}

func (h *URLHandler) GetAllByUserID(w http.ResponseWriter, r *http.Request) {
	tracer := tracing.NewTracer(tracing.GetTracer("url-handler"))
	ctx, span := tracer.StartServerSpan(r.Context(), "GetAllByUserID")
	defer span.End()

	urls, err := h.svc.GetAllByUserID(ctx)
	if err != nil {
		tracer.RecordError(span, err)
		h.logger.Error("GetAllByUserID failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(urls)
}

func (h *URLHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tracer := tracing.NewTracer(tracing.GetTracer("url-handler"))
	ctx, span := tracer.StartServerSpan(r.Context(), "GetByID")
	defer span.End()

	id := chi.URLParam(r, "id")
	url, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.IsNotFound(err) {
			h.logger.Warn("URL not found", "id", id)
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			tracer.RecordError(span, err)
			h.logger.Error("GetByID failed", "id", id, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	json.NewEncoder(w).Encode(url)
}

func (h *URLHandler) Add(w http.ResponseWriter, r *http.Request) {
	tracer := tracing.NewTracer(tracing.GetTracer("url-handler"))
	ctx, span := tracer.StartServerSpan(r.Context(), "Add")
	defer span.End()

	var url model.URL
	if err := json.NewDecoder(r.Body).Decode(&url); err != nil {
		tracer.RecordError(span, err)
		h.logger.Warn("Invalid request body for Add")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	url.Status = model.StatusUnknown

	if err := h.svc.Add(ctx, url); err != nil {
		if errors.IsInternal(err) {
			h.logger.Warn("Duplicate or invalid Add", "url", url, "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			tracer.RecordError(span, err)
			h.logger.Error("Add failed", "url", url, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *URLHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	tracer := tracing.NewTracer(tracing.GetTracer("url-handler"))
	ctx, span := tracer.StartServerSpan(r.Context(), "UpdateStatus")
	defer span.End()

	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		tracer.RecordError(span, err)
		h.logger.Warn("Invalid request body for UpdateStatus", "id", id)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateStatus(ctx, id, body.Status); err != nil {
		if errors.IsNotFound(err) {
			h.logger.Warn("URL not found for update", "id", id)
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			tracer.RecordError(span, err)
			h.logger.Error("UpdateStatus failed", "id", id, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}
