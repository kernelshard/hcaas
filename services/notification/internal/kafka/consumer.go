package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/IBM/sarama"

	"github.com/kernelshard/hcaas/services/notification/internal/model"
	"github.com/kernelshard/hcaas/services/notification/internal/service"
)

// Consumer is responsible for handling Kafka message consumption from a topic using a consumer group.
type Consumer struct {
	topic           string
	notificationSvc service.NotificationService
	consumerGroup   sarama.ConsumerGroup
	log             *slog.Logger
}

// NewKafkaConsumer constructs a new Kafka Consumer.
// It receives its consumer group via dependency injection.
func NewKafkaConsumer(
	topic string,
	consumerGroup sarama.ConsumerGroup,
	notificationSvc service.NotificationService,
	log *slog.Logger,
) *Consumer {
	return &Consumer{
		topic:           topic,
		consumerGroup:   consumerGroup,
		notificationSvc: notificationSvc,
		log:             log,
	}
}

// Start begins the Kafka consumer loop, listening for messages on the configured topic.
// It will block until the context is canceled or the consumer group is closed.
func (c *Consumer) Start(ctx context.Context) error {
	defer func() {
		if err := c.consumerGroup.Close(); err != nil {
			c.log.Warn("Failed to close consumer group", slog.Any("error", err))
		}
	}()

	c.log.Info("Kafka consumer started", slog.String("topic", c.topic))

	backoff := 1 * time.Second
	for {
		// Consume blocks until an error occurs or context is cancelled.
		err := c.consumerGroup.Consume(ctx, []string{c.topic}, c)
		if err != nil {
			c.log.Error("Error consuming messages", slog.Any("error", err))

			// Exit if consumer group is closed
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return err
			}

			// Back off on transient errors
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}

		if ctx.Err() != nil {
			c.log.Info("Context cancelled, stopping consumer")
			return ctx.Err()
		}
	}
}

// Setup is called once when a new consumer session starts.
// It's a good place to log which partitions this instance is assigned to.
func (c *Consumer) Setup(session sarama.ConsumerGroupSession) error {
	for topic, partitions := range session.Claims() {
		c.log.Info("Partition assignment",
			slog.String("topic", topic),
			slog.Any("partitions", partitions),
		)
	}
	return nil
}

// Cleanup is called once when the consumer session ends (rebalance, shutdown, etc).
func (c *Consumer) Cleanup(_ sarama.ConsumerGroupSession) error {
	c.log.Info("Kafka session cleanup complete")
	return nil
}

// ConsumeClaim is where the actual message consumption and processing happens.
// Kafka calls this method for each assigned partition.
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	// fetches message & send to business logic
	for message := range claim.Messages() {
		c.log.Debug("Message received",
			slog.String("topic", message.Topic),
			slog.Int("partition", int(message.Partition)),
			slog.Int64("offset", message.Offset),
		)

		// Parse the message
		var notif model.Notification
		if err := json.Unmarshal(message.Value, &notif); err != nil {
			c.log.Error("Failed to decode message", slog.Any("error", err))
			// skip the gibberish messages
			session.MarkMessage(message, "")
			continue
		}

		/*
		 NOTE: This is the core business logic call
		*/
		if err := c.notificationSvc.Send(session.Context(), &notif); err != nil {
			c.log.Error("Notification handling failed", slog.Any("error", err))
			continue
		}

		// Mark the message as processed (committed offset)
		session.MarkMessage(message, "")
	}
	return nil
}
