package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/samims/hcaas/services/auth/internal/service"
)

type key string

const (
	contextUserIDKey key = "user_id"
	contextEmailKey
)

func UserIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(contextUserIDKey).(string)
	return uid, ok
}

func AuthMiddleware(tokenService service.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "missing or malformed token", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			userID, email, err := tokenService.ValidateToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), contextUserIDKey, userID)
			ctx = context.WithValue(ctx, contextEmailKey, email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
