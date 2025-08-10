package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"go.opentelemetry.io/otel/attribute"

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/url/internal/model"
)

// NotificationProducer defines the interface for Kafka publishing
type NotificationProducer interface {
	Start(ctx context.Context)
	Publish(ctx context.Context, notif model.Notification) error
	Close(ctx context.Context)
}

type producer struct {
	asyncProducer sarama.AsyncProducer
	topic         string
	log           *slog.Logger
	wg            *sync.WaitGroup
	closeOnce     sync.Once
	tracer        *tracing.Tracer
}

// NewProducer uses DI to inject AsyncProducer, logger, topic, WaitGroup, and tracer.
func NewProducer(asyncProducer sarama.AsyncProducer, topic string, log *slog.Logger, wg *sync.WaitGroup, tracer *tracing.Tracer) NotificationProducer {
	if asyncProducer == nil || log == nil || wg == nil || tracer == nil {
		panic("NewProducer: nil dependencies provided")
	}
	if topic == "" {
		panic("NewProducer: topic must not be empty")
	}
	return &producer{
		asyncProducer: asyncProducer,
		topic:         topic,
		log:           log,
		wg:            wg,
		tracer:        tracer,
	}
}

// Start launches background handlers for success and error channels
func (p *producer) Start(ctx context.Context) {
	p.log.Info("Starting Kafka producer handlers")
	p.wg.Add(2)
	go p.handleSuccess(ctx)
	go p.handleErrors(ctx)
}

// handleSuccess logs successful deliveries
func (p *producer) handleSuccess(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case msg, ok := <-p.asyncProducer.Successes():
			if !ok {
				p.log.Info("Kafka successes channel closed")
				return
			}

			key, _ := msg.Key.Encode()
			p.log.Info("Message delivered",
				slog.String("topic", msg.Topic),
				slog.Int64("offset", msg.Offset),
				slog.String("key", string(key)))
		case <-ctx.Done():
			p.log.Info("Kafka success handler stopped by context")
			return
		}
	}
}

// handleErrors logs failed deliveries
func (p *producer) handleErrors(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case err, ok := <-p.asyncProducer.Errors():
			if !ok {
				p.log.Info("Kafka errors channel closed")
				return
			}
			p.log.Error("Message delivery failed",
				slog.String("topic", err.Msg.Topic),
				slog.Any("error", err.Err))
		case <-ctx.Done():
			p.log.Info("Kafka error handler stopped by context")
			return
		}
	}
}

// Publish sends a notification to the Kafka topic with tracing and context propagation
func (p *producer) Publish(ctx context.Context, notif model.Notification) error {
	ctx, span := p.tracer.StartClientSpan(ctx, "KafkaPublish")
	defer span.End()

	p.log.Info("Kafka publish called ")
	data, err := json.Marshal(notif)
	if err != nil {
		p.log.Error("Failed to marshal notification",
			slog.Any("notification", notif),
			slog.Any("error", err))
		span.RecordError(err)
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Inject trace context into headers for propagation to consumer
	headers := tracing.InjectTraceContext(ctx, nil)

	msg := &sarama.ProducerMessage{
		Topic:     p.topic,
		Key:       sarama.StringEncoder(notif.UrlID),
		Value:     sarama.ByteEncoder(data),
		Timestamp: time.Now(),
		Headers:   headers,
	}

	select {
	case p.asyncProducer.Input() <- msg:
		p.log.Info("Message queued to Kafka",
			slog.String("topic", p.topic),
			slog.String("key", notif.UrlID),
			slog.Any("notification", notif))
		span.SetAttributes(
			attribute.String("kafka.topic", p.topic),
			attribute.String("kafka.key", notif.UrlID),
			attribute.String("notification.type", notif.Type),
		)
		return nil
	case <-ctx.Done():
		p.log.Warn("Publish cancelled by context",
			slog.String("url_id", notif.UrlID))
		span.SetStatus(2, "Publish cancelled by context") // 2 = Error
		return ctx.Err()
	}
}

// Close shuts down the producer and waits for workers
func (p *producer) Close(_ context.Context) {
	p.closeOnce.Do(func() {
		p.log.Info("Closing Kafka producer...")
		p.asyncProducer.AsyncClose()
		p.wg.Wait()
		p.log.Info("Kafka producer closed")
	})
}
