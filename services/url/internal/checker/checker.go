package checker

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/url/internal/kafka"
	"github.com/samims/hcaas/services/url/internal/metrics"
	"github.com/samims/hcaas/services/url/internal/model"
	"github.com/samims/hcaas/services/url/internal/service"
)

const (
	Healthy   = "healthy"
	UnHealthy = "unhealthy"
)

type URLChecker struct {
	svc                  service.URLService
	logger               *slog.Logger
	httpClient           *http.Client
	interval             time.Duration
	notificationProducer kafka.NotificationProducer
	tracer               *tracing.Tracer
	concurrencyLimit     int
	httpTimeOut          time.Duration
}

func NewURLChecker(
	svc service.URLService,
	logger *slog.Logger,
	client *http.Client,
	interval time.Duration,
	producer kafka.NotificationProducer,
	tracer *tracing.Tracer,
	concurrencyLimit int,
) *URLChecker {
	if producer == nil {
		// This panic indicates a serious configuration error that should be caught
		panic("NewURLChecker: notificationProducer cannot be nil")
	}
	if tracer == nil {
		panic("NewURLChecker: tracer cannot be nil")
	}
	return &URLChecker{
		svc:                  svc,
		logger:               logger,
		httpClient:           client,
		interval:             interval,
		notificationProducer: producer,
		tracer:               tracer,
		concurrencyLimit:     concurrencyLimit,
	}
}

func (uc *URLChecker) Start(ctx context.Context) {
	uc.logger.Info("URLChecker started")

	ticker := time.NewTicker(uc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			uc.logger.Info("URLChecker stopped")
			return
		case <-ticker.C:
			uc.CheckAllURLs(ctx)
		}
	}
}

func (uc *URLChecker) CheckAllURLs(ctx context.Context) {
	ctx, span := uc.tracer.StartServerSpan(ctx, "CheckAllURLs")
	defer span.End()

	urls, err := uc.svc.GetAll(ctx)
	if err != nil {
		uc.logger.Error("Failed to fetch URLs", slog.Any("error", err))
		uc.tracer.RecordError(span, err)
		return
	}

	var wg sync.WaitGroup
	// Use the configurable concurrency limit for the semaphore.
	sem := make(chan struct{}, uc.concurrencyLimit)

	for _, url := range urls {
		wg.Add(1)
		go func(url model.URL) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, span := uc.tracer.StartClientSpan(ctx, "CheckURL")
			defer span.End()

			uc.logger.Info("Checking URL", slog.String("id", url.ID), slog.String("address", url.Address))

			status := uc.ping(ctx, url.Address)
			uc.logger.Info("After ping", slog.String("url_id", url.ID), slog.Any("address", url.Address), slog.String("status", status))

			span.SetAttributes(
				attribute.String("url.id", url.ID),
				attribute.String("url.address", url.Address),
				attribute.String("url.status", status),
			)

			err := uc.svc.UpdateStatus(ctx, url.ID, status)
			if err != nil {
				uc.logger.Error("Failed to update URL status",
					slog.String("urlID", url.ID),
					slog.String("status", status),
					slog.Any("error", err),
				)
				uc.tracer.RecordError(span, err)
			} else {
				uc.logger.Info("URL status updated",
					slog.String("urlID", url.ID),
					slog.String("address", url.Address),
					slog.String("status", status),
				)

				if status == UnHealthy {
					notification := model.Notification{
						UrlID:     url.ID,
						Type:      "url_unhealthy",
						Message:   "URL is unhealthy: " + url.Address,
						Status:    "pending",
						CreatedAt: time.Now(),
					}

					if err := uc.notificationProducer.Publish(ctx, notification); err != nil {
						uc.logger.Error("Failed to publish notification",
							slog.String("url_id", url.ID),
							slog.Any("error", err))
						uc.tracer.RecordError(span, err)
					}
				}
			}
		}(url)
	}

	wg.Wait()
}

// ping performs a HTTP GET with timeout and metrics
func (uc *URLChecker) ping(parentCtx context.Context, target string) string {
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		uc.logger.Warn("Failed to create HTTP request", slog.String("address", target), slog.Any("error", err))
		metrics.URLCheckStatus.WithLabelValues(model.StatusDown).Inc()
		return UnHealthy
	}

	start := time.Now()
	resp, err := uc.httpClient.Do(req)
	duration := time.Since(start).Seconds()

	if err != nil {
		uc.logger.Warn("HTTP request failed", slog.String("address", target), slog.Any("error", err))
		metrics.URLCheckStatus.WithLabelValues(model.StatusDown).Inc()
		metrics.URLCheckDuration.WithLabelValues(model.StatusDown).Observe(duration)
		return UnHealthy
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		uc.logger.Warn("Unhealthy HTTP status code",
			slog.String("address", target),
			slog.Int("statusCode", resp.StatusCode),
		)
		metrics.URLCheckStatus.WithLabelValues(model.StatusDown).Inc()
		metrics.URLCheckDuration.WithLabelValues(model.StatusDown).Observe(duration)
		return UnHealthy
	}

	metrics.URLCheckStatus.WithLabelValues(model.StatusUP).Inc()
	metrics.URLCheckDuration.WithLabelValues(model.StatusUP).Observe(duration)
	return Healthy
}
