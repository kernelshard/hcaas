package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/joho/godotenv"

	"github.com/samims/hcaas/services/url/internal/checker"
	"github.com/samims/hcaas/services/url/internal/config"
	"github.com/samims/hcaas/services/url/internal/handler"
	"github.com/samims/hcaas/services/url/internal/kafka"
	"github.com/samims/hcaas/services/url/internal/logger"
	"github.com/samims/hcaas/services/url/internal/metrics"
	"github.com/samims/hcaas/services/url/internal/router"
	"github.com/samims/hcaas/services/url/internal/service"
	"github.com/samims/hcaas/services/url/internal/storage"
	"github.com/samims/otelkit"
)

const (
	serviceName = "url-service"
	// collectorEndpoint = "otel-collector:4371" //mEnsure this matches your docker-compose setup
	// collectorEndpoint = "hcaas_jaeger_all_in_one:4317"
)

func main() {

	// Setup signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	l := logger.NewLogger()
	slog.SetDefault(l)

	metrics.Init()

	if err := godotenv.Load(); err != nil {
		l.Error("Error loading .env file", "err", err)
	}

	// Load configuration from environment variables
	cfg, err := config.LoadConfig()
	if err != nil {
		l.Error("Failed to load configuration", "err", err)
		os.Exit(1)
	}

	// Setup OpenTelemetry tracing with custom provider configuration
	tracingConfig := otelkit.NewProviderConfig(serviceName, "v1.0.0").
		WithOTLPExporter(cfg.OTLPConfig.Endpoint, cfg.OTLPConfig.Protocol, cfg.OTLPConfig.Insecure).
		WithSampling("probabilistic", 0.1).                        // 10% sampling rate
		WithBatchOptions(2*time.Second, 10*time.Second, 512, 2048) // Optimized batch settings

	provider, err := otelkit.NewProvider(ctx, tracingConfig)
	if err != nil {
		l.Error("Failed to initialize OpenTelemetry tracing provider",
			slog.String("error", err.Error()),
			slog.String("service", serviceName))
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

	tracer := otelkit.New(serviceName)

	dbPool, err := storage.NewPostgresPool(ctx, cfg.DBConfig.URL)
	if err != nil {
		l.Error("Failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Initialize layers
	ps := storage.NewPostgresStorage(dbPool, tracer)
	urlSvc := service.NewURLService(ps, l, tracer)
	healthSvc := service.NewHealthService(ps, l)

	// Kafka producers setup
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll // Acks from all replicas
	saramaConfig.Producer.Retry.Max = 5
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.ClientID = "url-service-producer"

	kafkaAsyncProducer, err := sarama.NewAsyncProducer(cfg.KafkaConfig.Brokers, saramaConfig)
	if err != nil {
		l.Error("Failed to create sarama producer", slog.Any("error", err))
		os.Exit(1)
	}

	var wg sync.WaitGroup

	l.Info("Before NewProducer")
	notificationProducer := kafka.NewProducer(kafkaAsyncProducer, cfg.KafkaConfig.NotifTopic, l, &wg, tracer)
	l.Info("After NewProducer")

	l.Info("Calling notificationProducer.Start()")
	notificationProducer.Start(ctx)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	concurrencyLimit := 10
	chkr := checker.NewURLChecker(urlSvc, l, httpClient, 1*time.Minute, notificationProducer, tracer, concurrencyLimit)
	go chkr.Start(ctx)

	urlHandler := handler.NewURLHandler(urlSvc, l, tracer)
	healthHandler := handler.NewHealthHandler(healthSvc, l)

	// Setup router and server
	r := router.NewRouter(urlHandler, healthHandler, l, serviceName)
	// Apply OpenTelemetry HTTP server middleware to the router.
	// This will automatically create spans for incoming requests and propagate context.
	// Pass the service name to the middleware

	server := &http.Server{
		Addr:    ":" + cfg.AppCfg.Port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		l.Info("Server started", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.Error("Failed to start server", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	l.Info("Shutting down server...")

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxTimeout); err != nil {
		l.Error("Shutdown failed", "err", err)
	} else {
		l.Info("Server exited cleanly")
	}
}
