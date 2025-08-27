package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/samims/otelkit"
	"go.opentelemetry.io/otel/attribute"

	"github.com/samims/hcaas/services/auth/internal/service"
)

const (
	KeyError = "error"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	authSvc service.AuthService
	logger  *slog.Logger
	tracer  *otelkit.Tracer
}

func NewAuthHandler(authSvc service.AuthService, logger *slog.Logger, tracer *otelkit.Tracer) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, logger: logger, tracer: tracer}
}

// inline error responder
func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{KeyError: message})
}

// Register handles User Registration/Signup
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), "auth_handler.Register")
	defer span.End()
	span.SetAttributes(
		attribute.String("handler.component", "auth_handler"),
	)
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		otelkit.RecordError(span, err)
		h.logger.Warn("Invalid register payload", slog.String("error", err.Error()))
		respondError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	user, err := h.authSvc.Register(ctx, req.Email, req.Password)
	if err != nil {
		otelkit.RecordError(span, err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), "auth_handler.Login")
	defer span.End()
	span.SetAttributes(
		attribute.String("operation", "user_login"),
		attribute.String("handler.component", "auth_handler"),
	)
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		otelkit.RecordError(span, err)
		respondError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	_, token, err := h.authSvc.Login(ctx, req.Email, req.Password)
	if err != nil {
		otelkit.RecordError(span, err)
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (h *AuthHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartServerSpan(r.Context(), "auth_handler.GetUser")
	defer span.End()
	span.SetAttributes(
		attribute.String("operation", "get_user"),
		attribute.String("handler.component", "auth_handler"),
	)
	h.logger.Info("Get User handler")
	email := r.URL.Query().Get("email")

	if email == "" {
		http.Error(w, "missing email query param", http.StatusBadRequest)
		return
	}
	user, err := h.authSvc.GetUserByEmail(ctx, email)

	if err != nil {
		otelkit.RecordError(span, err)
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(user)

}

func (h *AuthHandler) Validate(w http.ResponseWriter, r *http.Request) {
	_, span := h.tracer.StartServerSpan(r.Context(), "auth_handler.Validate")
	defer span.End()
	span.SetAttributes(
		attribute.String("operation", "validate_token"),
		attribute.String("handler.component", "auth_handler"),
	)
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		respondError(w, http.StatusUnauthorized, "missing token")
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	userID, email, err := h.authSvc.ValidateToken(token)
	if err != nil {
		otelkit.RecordError(span, err)
		respondError(w, http.StatusUnauthorized, "invalid token")
	}

	resp := struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"` // Alternative field name
	}{
		UserID: userID,
		Email:  email, // Set both fields for backward compatibility
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		otelkit.RecordError(span, err)
		h.logger.Error("Failed to encode validation response",
			slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
