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

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/url/internal/checker"
	"github.com/samims/hcaas/services/url/internal/handler"
	"github.com/samims/hcaas/services/url/internal/kafka"
	"github.com/samims/hcaas/services/url/internal/logger"
	"github.com/samims/hcaas/services/url/internal/metrics"
	"github.com/samims/hcaas/services/url/internal/router"
	"github.com/samims/hcaas/services/url/internal/service"
	"github.com/samims/hcaas/services/url/internal/storage"
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

	// Create a new tracing configuration from environment variables.
	tracerCfg := tracing.NewConfig()
	if err := tracerCfg.Validate(); err != nil {
		l.Error("Invalid tracing config", slog.Any("error", err))
		os.Exit(1)
	}

	// ---- OpenTelemetry Tracing Setup ----
	tracerShutdown, err := tracing.SetupTracing(ctx, l)
	if err != nil {
		l.Error("Failed to initialize OpenTelemetry TracerProvider", slog.Any("error", err))
		os.Exit(1)
	}

	// IMPORTANT: Defer the tracer shutdown function to ensure all spans are flushed
	// before the application exits.
	defer tracerShutdown(context.Background())
	// --- End OpenTelemetry Tracing Setup ---

	dbPool, err := storage.NewPostgresPool(ctx)
	if err != nil {
		l.Error("Failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Initialize layers
	tracer := tracing.NewTracer(tracing.GetTracer(serviceName))
	ps := storage.NewPostgresStorage(dbPool, tracer)
	urlSvc := service.NewURLService(ps, l, tracer)
	healthSvc := service.NewHealthService(ps, l)

	// Kafka producers setup
	// TODO: will move to another place
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	kafkaNotifTopic := os.Getenv("KAFKA_NOTIF_TOPIC")
	if kafkaBrokers == "" || kafkaNotifTopic == "" {
		l.Error("KAFKA_BROKERS or KAFKA_TOPIC not set")
		os.Exit(1)
	}

	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll // Acks from all replicas
	saramaConfig.Producer.Retry.Max = 5
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.ClientID = "url-service-producer"

	kafkaAsyncProducer, err := sarama.NewAsyncProducer([]string{kafkaBrokers}, saramaConfig)

	if err != nil {
		l.Error("Failed to create sarama producer", slog.Any("error", err))
		os.Exit(1)
	}

	var wg sync.WaitGroup

	l.Info("Before NewProducer")
	notificationProducer := kafka.NewProducer(kafkaAsyncProducer, kafkaNotifTopic, l, &wg, tracer)
	l.Info("After NewProducer")

	l.Info("Calling notificationProducer.Start()")

	notificationProducer.Start(ctx)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	concurrencyLimit := 10
	chkr := checker.NewURLChecker(urlSvc, l, httpClient, 1*time.Minute, notificationProducer, tracer, concurrencyLimit)
	go chkr.Start(ctx)

	urlHandler := handler.NewURLHandler(urlSvc, l)
	healthHandler := handler.NewHealthHandler(healthSvc, l)

	// Setup router and server
	port := ":8080"

	r := router.NewRouter(urlHandler, healthHandler, l, serviceName)
	// Apply OpenTelemetry HTTP server middleware to the router.
	// This will automatically create spans for incoming requests and propagate context.
	// Pass the service name to the middleware

	server := &http.Server{
		Addr:    port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		l.Info("Server started", "addr", port)
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
