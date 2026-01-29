package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/samims/otelkit"

	"github.com/kernelshard/hcaas/services/auth/internal/config"
	"github.com/kernelshard/hcaas/services/auth/internal/handler"
	"github.com/kernelshard/hcaas/services/auth/internal/logger"
	customMiddleware "github.com/kernelshard/hcaas/services/auth/internal/middleware"
	"github.com/kernelshard/hcaas/services/auth/internal/service"
	"github.com/kernelshard/hcaas/services/auth/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()
	l := logger.NewJSONLogger()

	err := godotenv.Load()
	if err != nil {
		l.Error("Failed to load environment variables", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		l.Error("Failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Setup OpenTelemetry tracing with custom provider configuration
	// Use OTLP configuration from environment variables via config
	tracingConfig := otelkit.NewProviderConfig("auth-service", "v1.0.0").
		WithOTLPExporter(cfg.OTLPConfig.Endpoint, cfg.OTLPConfig.Protocol, cfg.OTLPConfig.Insecure).
		WithSampling("probabilistic", 0.1).                        // 10% sampling rate
		WithBatchOptions(2*time.Second, 10*time.Second, 512, 2048) // Optimized batch settings

	provider, err := otelkit.NewProvider(ctx, tracingConfig)
	if err != nil {
		l.Error("Failed to initialize OpenTelemetry tracing provider",
			slog.String("error", err.Error()),
			slog.String("service", "auth-service"))
		os.Exit(1)
	}

	// Graceful shutdown with error handling
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := provider.Shutdown(shutdownCtx); err != nil {
			l.Error("Failed to gracefully shutdown tracing provider",
				slog.String("error", err.Error()))
		} else {
			l.Info("Tracing provider shutdown completed successfully")
		}
	}()

	tracer := otelkit.New("auth-service")

	dbPool, err := storage.NewPostgresPool(ctx, cfg.DBConfig.URL)
	if err != nil {
		l.Error("Failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer dbPool.Close()

	userStorage := storage.NewUserStorage(dbPool, tracer)

	tokenSvc := service.NewJWTService(cfg.SecretKey, cfg.AuthExpiry, l, tracer)
	authSvc := service.NewAuthService(userStorage, l, tokenSvc, tracer)
	healthSvc := service.NewHealthService(userStorage, l)

	authHandler := handler.NewAuthHandler(authSvc, l, tracer)
	healthHandler := handler.NewHealthHandler(healthSvc, l)

	r := chi.NewRouter()
	r.Use(otelkit.NewHttpMiddleware(tracer).Middleware)
	r.Use(customMiddleware.MetricsMiddleware)

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// public
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Get("/validate", authHandler.Validate)
	})

	// protected
	r.Group(func(r chi.Router) {
		r.Use(customMiddleware.AuthMiddleware(tokenSvc))
		r.Get("/me", authHandler.GetUser)
	})

	r.Get("/readyz", healthHandler.Readiness)
	r.Get("/healthz", healthHandler.Liveness)

	r.Handle("/metrics", promhttp.Handler())

	port := ":8081"

	server := &http.Server{Addr: port, Handler: r}

	go func() {
		l.Info("Server started", "addr", port)
		if err := server.ListenAndServe(); err != nil {
			l.Error("Failed to start server", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, os.Interrupt)
	<-quit
	l.Info("Shutting down server")
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxTimeout); err != nil {
		l.Error("Shutdown failed", "err", err)
	} else {
		l.Info("Server exited cleanly")
	}

}
