package middleware

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kernelshard/hcaas/services/url/internal/model"
)

func AuthMiddleware(authServiceURL string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				logger.Warn("Unauthorized: missing or malformed token")
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")

			validateURL := authServiceURL + "auth/validate"
			logger.Debug("Calling auth service validation endpoint",
				"url", validateURL,
				"method", r.Method,
				"path", r.URL.Path)

			req, err := http.NewRequest(http.MethodGet, validateURL, nil)
			if err != nil {
				logger.Error("Failed to create request to auth service", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			req.Header.Set("Authorization", "Bearer "+token)
			client := http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				logger.Error("Failed to call auth service", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				logger.Warn("Auth service validation failed", "status", resp.StatusCode, "body", string(bodyBytes))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error("Failed to read auth response body", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			logger.Debug("Auth service response", "body", string(bodyBytes))

			var authResponse struct {
				UserID string `json:"user_id"`
				Email  string `json:"email"`
			}

			if err := json.Unmarshal(bodyBytes, &authResponse); err != nil {
				logger.Error("Failed to decode auth service response",
					"error", err,
					"response", string(bodyBytes))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if authResponse.UserID == "" {
				logger.Error("No user identifier found in auth response",
					slog.String("response", string(bodyBytes)))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if authResponse.Email == "" {
				logger.Error("No email found in auth response", slog.String("response", string(bodyBytes)))
			}

			ctx := context.WithValue(r.Context(), model.ContextUserIDKey, authResponse.UserID)
			ctx = context.WithValue(ctx, model.ContextEmailKey, authResponse.Email)
			logger.Info("User authenticated",
				"user_id", authResponse.UserID,
				"method", r.Method,
				"path", r.URL.Path)

			// Verify context value is set correctly
			if ctx.Value(model.ContextUserIDKey) == nil {
				logger.Error("Failed to set user_id in context")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
