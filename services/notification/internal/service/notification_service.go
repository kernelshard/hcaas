package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"

	"github.com/samims/hcaas/services/notification/internal/model"
	"github.com/samims/hcaas/services/notification/internal/store"
	"github.com/samims/otelkit"
)

// NotificationService defines behavior for sending notifications
// Implementations may enqueue or directly send notifications
type NotificationService interface {
	// Start the background workers (for async delivery)
	Start(ctx context.Context) error
	// Send queues or sends a notification
	Send(ctx context.Context, n *model.Notification) error
}

// notificationService is the default implementation of NotificationService
type notificationService struct {
	store       store.NotificationStorage
	delivery    DeliveryService
	workerLimit int
	interval    time.Duration
	l           *slog.Logger
	tracer      *otelkit.Tracer
}

// NewNotificationService creates a new notification service instance
func NewNotificationService(
	store store.NotificationStorage,
	delivery DeliveryService,
	workerLimit int,
	interval time.Duration,
	logger *slog.Logger,
	tracer *otelkit.Tracer,
) NotificationService {
	return &notificationService{
		store:       store,
		delivery:    delivery,
		workerLimit: workerLimit,
		interval:    interval,
		l:           logger,
		tracer:      tracer,
	}
}

// Send sends the notification
func (s *notificationService) Send(ctx context.Context, n *model.Notification) error {
	ctx, span := s.tracer.StartClientSpan(ctx, "notificationService.Send")
	defer span.End()

	if n == nil {
		err := fmt.Errorf("notification cannot be nil")
		span.RecordError(err)
		return err
	}
	// Simulate sending notification service
	s.l.Info("Notification service send called with url ", slog.String("url_id", n.UrlId))
	n.Status = model.StatusPending
	n.CreatedAt = time.Now()
	n.UpdatedAt = n.CreatedAt

	span.SetAttributes(attribute.String("notification.url_id", n.UrlId))
	span.SetAttributes(attribute.String("notification.status", string(n.Status)))

	s.l.Info("Queuing new notification for processing", slog.String("url_id", n.UrlId))

	if err := s.store.Save(ctx, n); err != nil {
		span.RecordError(err)
		s.l.Error("Failed to save notification to store", slog.String("url_id", n.UrlId), slog.Any("error", err))
		return err
	}
	return nil
}

// Start begins periodic processing of queued notifications
func (s *notificationService) Start(ctx context.Context) error {
	s.l.InfoContext(ctx, "Starting notification worker", slog.Int("max_workers", s.workerLimit))
	var err error
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.l.InfoContext(ctx, "Notification service shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err = s.processBatch(ctx); err != nil {
				s.l.Error("Error processing notification batch", slog.Any("error", err))
			}
		}
	}
}

// processBatch fetches ending notifications and processes them concurrently
func (s *notificationService) processBatch(ctx context.Context) error {
	// Fetch all pending notifications from the store layer.
	notifs, err := s.store.GetPending(ctx)
	if err != nil {
		s.l.ErrorContext(ctx, "Error fetching pending notifications from store", slog.Any("error", err))
		return err
	}
	// If no notifications are pending, we exit early
	if len(notifs) == 0 {
		s.l.InfoContext(ctx, "No pending notifications to process")
		return nil
	}

	s.l.InfoContext(ctx, "Processing batch of pending notifications", slog.Int("count", len(notifs)))

	// Create an error group to manage concurrent goroutines and collect their errors.
	eg, ctx := errgroup.WithContext(ctx)
	// Create a buffered channel to act as a semaphore, limiting concurrency to s.workerLimit.
	sem := make(chan struct{}, s.workerLimit)

	// Iterate through each notification in the batch.
	for _, notif := range notifs {
		// Create a local variable to avoid issues with goroutine over loop variables.
		notif := notif
		// Acquire a token from the semaphore, blocking if the worker limit is reached
		sem <- struct{}{}
		// Start a new goroutine for each notification.
		eg.Go(func() error {
			// Release the token back to the semaphore when the goroutine finishes.
			defer func() { <-sem }()
			// process the notification
			return s.processNotification(ctx, &notif)
		})
	}
	return eg.Wait()
}

// processNotification handles delivery and updates status
func (s *notificationService) processNotification(ctx context.Context, n *model.Notification) error {
	ctx, span := s.tracer.StartClientSpan(ctx, "notificationService.processNotification")
	defer span.End()

	if n == nil {
		err := fmt.Errorf("notification cannot be nil")
		span.RecordError(err)
		return err
	}

	span.SetAttributes(attribute.Int("notification.id", n.ID))
	span.SetAttributes(attribute.String("notification.url_id", n.UrlId))

	start := time.Now()
	s.l.InfoContext(ctx, "Attempting to deliver notification", slog.Int("id", n.ID), slog.String("url_id", n.UrlId))

	if err := s.delivery.Deliver(ctx, n); err != nil {
		span.RecordError(err)
		s.l.ErrorContext(ctx, "Notification delivery failed", slog.Int("id", n.ID), slog.String("url_id", n.UrlId), slog.Any("error", err))
		updateErr := s.store.UpdateStatus(ctx, n.ID, model.StatusFailed)
		if updateErr != nil {
			span.RecordError(updateErr)
			s.l.ErrorContext(ctx, "Failed to update status to failed after delivery error", slog.Int("id", n.ID), slog.Any("delivery_error", err), slog.Any("update_error", updateErr))
			return fmt.Errorf("notification delivery failed: %w; status update to 'failed' also failed: %w", err, updateErr)
		}
		return err
	}

	duration := time.Since(start)
	s.l.InfoContext(ctx, "Notification delivery succeeded", slog.Int("id", n.ID), slog.String("url_id", n.UrlId), slog.Duration("duration", duration))
	span.SetAttributes(attribute.String("notification.status", string(model.StatusSent)))
	span.SetAttributes(attribute.Int64("notification.duration_ms", duration.Milliseconds()))

	updateErr := s.store.UpdateStatus(ctx, n.ID, model.StatusSent)
	if updateErr != nil {
		span.RecordError(updateErr)
		s.l.Error("Failed to update status to sent after successful delivery", slog.Int("id", n.ID), slog.Any("error", updateErr))
		return updateErr
	}
	return nil
}
