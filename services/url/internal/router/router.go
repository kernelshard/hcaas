package router

import (
	"net/http"
	"os"
	"time"

	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/samims/hcaas/services/url/internal/handler"
	customMiddleware "github.com/samims/hcaas/services/url/internal/middleware"
)

func NewRouter(h *handler.URLHandler, healthHandler *handler.HealthHandler, logger *slog.Logger, serviceName string) http.Handler {
	r := chi.NewRouter()
	authSvcURL := os.Getenv("AUTH_SVC_URL")
	authMiddleware := customMiddleware.AuthMiddleware(authSvcURL, logger)

	// Middleware
	r.Use(customMiddleware.MetricsMiddleware)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Add OpenTelemetry middleware
	r.Use(func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, serviceName)
	})

	r.Route("/urls", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.GetAll)
		r.Get("/{id}", h.GetByID)
		r.Get("/me", h.GetAllByUserID)
		r.Post("/", h.Add)
	})

	// Health & Readiness Routes
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/readyz", healthHandler.Readiness)
	r.Handle("/metrics", promhttp.Handler())

	return r
}
