package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	_ "github.com/lib/pq"
	"github.com/samims/otelkit"

	"github.com/samims/hcaas/services/notification/internal/config"
	"github.com/samims/hcaas/services/notification/internal/handler"
	"github.com/samims/hcaas/services/notification/internal/kafka"
	"github.com/samims/hcaas/services/notification/internal/logger"
	"github.com/samims/hcaas/services/notification/internal/service"
	"github.com/samims/hcaas/services/notification/internal/store"
)

func main() {
	ctx := context.Background()

	// Load configuration from environment variables and exit on error.
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize the application logger.
	logr := logger.NewLogger()

	// Setup OpenTelemetry Tracing with custom provider configuration
	// Use OTLP configuration from environment variables via config
	tracingConfig := otelkit.NewProviderConfig("notification-service", "v1.0.0").
		WithOTLPExporter(cfg.OTLPConfig.Endpoint, cfg.OTLPConfig.Protocol, cfg.OTLPConfig.Insecure).
		WithSampling("probabilistic", 0.1).                        // 10% sampling rate
		WithBatchOptions(2*time.Second, 10*time.Second, 512, 2048) // Optimized batch settings

	provider, err := otelkit.NewProvider(ctx, tracingConfig)
	if err != nil {
		logr.Error("Failed to initialize OpenTelemetry TracerProvider", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Graceful shutdown with error handling
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := provider.Shutdown(shutdownCtx); err != nil {
			logr.Error("Failed to gracefully shutdown tracing provider", slog.String("error", err.Error()))
		} else {
			logr.Info("Tracing provider shutdown completed successfully")
		}
	}()

	// --- Dependency Injection Setup ---

	// The main function is responsible for creating a single, shared database connection pool.
	db, err := store.ConnectPostgres(cfg.DBConfig)
	if err != nil {
		logr.Error("failed to create postgres storage with database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close() // Ensure the connection is closed when main exits.

	// Now, inject this single database connection into the services that need it.
	dbStore, err := store.NewPostgresStorage(db)
	if err != nil {
		logr.Error("failed to create postgres storage instance", "error", err)
		os.Exit(1)
	}

	// Create tracer
	tracer := otelkit.New("notification-service")

	// Inject the store into the services that need it.
	healthSvc := service.NewHealthService(dbStore)
	delivery := service.NewDeliveryService(logr)
	notifSvc := service.NewNotificationService(
		dbStore, // The notification service now also depends on the store.
		delivery,
		cfg.WorkerLimit,
		cfg.WorkerInterval,
		logr,
		tracer,
	)

	// Setup Kafka consumer group with a shared configuration.
	saramaCfg := sarama.NewConfig()
	saramaCfg.Version = sarama.V2_1_0_0
	saramaCfg.Consumer.Return.Errors = true
	consumerGroup, err := sarama.NewConsumerGroup(
		cfg.ConsumerConfig.KafkaBrokers,
		cfg.ConsumerConfig.KafkaConsumerGroup,
		saramaCfg,
	)
	if err != nil {
		logr.Error("failed to create Kafka consumer group", "error", err)
		os.Exit(1)
	}

	// Create Kafka consumer, injecting the notification service as a handler.
	consumer := kafka.NewKafkaConsumer(
		cfg.ConsumerConfig.KafkaTopic,
		consumerGroup,
		notifSvc,
		logr,
	)

	// HTTP health handler and server
	hHandler := handler.NewHealthHandler(healthSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hHandler.HealthCheck)

	hServer := &http.Server{
		Addr:    ":" + cfg.AppCfg.Port,
		Handler: mux,
	}

	// Use a WaitGroup to gracefully shut down all goroutines.
	var wg sync.WaitGroup

	// Start notification worker in a separate goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := notifSvc.Start(context.Background()); err != nil && err != context.Canceled {
			logr.Error("notification worker stopped with error", "error", err)
		}
	}()

	// Start Kafka consumer in a separate goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := consumer.Start(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			logr.Error("Kafka consumer stopped with error", "error", err)
		}
	}()

	// Start HTTP server in a separate goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		logr.Info("Starting health server", "addr", hServer.Addr)
		if err := hServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logr.Error("Health server failed", "error", err)
		}
	}()

	// Wait for a termination signal from the OS.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logr.Info("Shutdown signal received")

	// Gracefully shut down the HTTP server.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hServer.Shutdown(ctx)

	// Wait for all worker goroutines to finish their work.
	wg.Wait()
	logr.Info("Service shut down gracefully")
}
