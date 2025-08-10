// Package main provides the entry point for the API Gateway service.
//
// This service acts as the single point of entry for all client requests,
// handling authentication, rate limiting, request routing, and observability.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/gateway/internal/config"
	"github.com/samims/hcaas/services/gateway/internal/handler"
	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
	gwMiddleware "github.com/samims/hcaas/services/gateway/internal/middleware"
	"github.com/samims/hcaas/services/gateway/internal/proxy"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	l := logger.New(cfg.Observability.ServiceName)
	l.Info("Starting API Gateway",
		logger.FieldService(cfg.Observability.ServiceName),
	)

	// Initialize tracing
	var tracerShutdown func(context.Context) error
	if cfg.Observability.TracingEnabled {
		// Create slog logger for tracing setup
		slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

		shutdown, err := tracing.SetupTracing(context.Background(), slogLogger)
		if err != nil {
			l.LogError(context.Background(), err, "Failed to setup tracing",
				logger.FieldService("tracing"),
			)
			// Continue without tracing rather than failing
		} else {
			tracerShutdown = shutdown
			l.Info("Tracing initialized successfully")
		}
	}

	// Initialize metrics
	metricsObj := metrics.New()

	// Initialize service proxy
	serviceProxy := proxy.NewServiceProxy(&cfg.Services, l, metricsObj)

	// Initialize middleware
	authMiddleware := gwMiddleware.NewAuthMiddleware(cfg.Security.JWTSecret, l, metricsObj)

	var rateLimitMiddleware *gwMiddleware.RateLimitMiddleware
	if cfg.RateLimit.Enabled {
		rateLimitMiddleware = gwMiddleware.NewRateLimitMiddleware(
			cfg.RateLimit.RequestsPerSecond,
			cfg.RateLimit.BurstSize,
			l,
			metricsObj,
		)
		defer rateLimitMiddleware.Stop()
	}

	// Initialize handlers
	healthHandler := handler.NewHealthHandler(l, &cfg.Services)
	gatewayHandler := handler.NewGatewayHandler(serviceProxy, l, metricsObj)

	// Setup router
	r := chi.NewRouter()

	// Add global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Add custom middleware
	r.Use(gwMiddleware.NewLoggingMiddleware(l, metricsObj).Middleware())
	r.Use(gwMiddleware.NewCORSMiddleware().Middleware())

	// Add tracing middleware if tracing is enabled
	if cfg.Observability.TracingEnabled {
		r.Use(gwMiddleware.NewTracingMiddleware(l).Middleware())
	}

	if cfg.RateLimit.Enabled {
		r.Use(rateLimitMiddleware.Middleware())
	}

	r.Use(authMiddleware.Middleware())

	// Health and metrics endpoints (public)
	r.Route("/", func(r chi.Router) {
		r.Get("/healthz", healthHandler.HealthCheck)
		r.Get("/readyz", healthHandler.ReadinessCheck)
		r.Get("/metrics", promhttp.Handler().ServeHTTP)
	})

	// API routes (proxied to upstream services)
	r.Route("/", func(r chi.Router) {
		r.HandleFunc("/*", gatewayHandler.ProxyHandler)
	})

	// Setup HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		l.Info("Starting HTTP server",
			logger.FieldService("http-server"),
			logger.FieldPath(server.Addr),
		)
		serverErrors <- server.ListenAndServe()
	}()

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		l.LogError(context.Background(), err, "Server startup failed")

	case sig := <-shutdown:
		l.Info("Shutdown signal received",
			logger.FieldService("shutdown"),
			logger.FieldUserID(sig.String()),
		)

		// Give outstanding requests time to complete
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		// Shutdown tracer first to flush remaining spans
		if tracerShutdown != nil {
			if err := tracerShutdown(ctx); err != nil {
				l.LogError(ctx, err, "Failed to shutdown tracer")
			} else {
				l.Info("Tracer shutdown completed")
			}
		}

		if err := server.Shutdown(ctx); err != nil {
			l.LogError(ctx, err, "Graceful shutdown failed")

			// Force shutdown
			if err := server.Close(); err != nil {
				l.LogError(ctx, err, "Force shutdown failed")
			}
		}
	}

	l.Info("API Gateway shutdown complete")
}
