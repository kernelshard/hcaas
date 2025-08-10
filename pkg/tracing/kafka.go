package tracing

import (
	"context"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel/propagation"
)

// InjectTraceContext injects OpenTelemetry trace context into Kafka message headers
// for propagation to downstream consumers.
func InjectTraceContext(ctx context.Context, headers []sarama.RecordHeader) []sarama.RecordHeader {
	carrier := propagation.MapCarrier{}
	propagator := propagation.TraceContext{}
	propagator.Inject(ctx, carrier)

	// Create new headers slice to avoid mutation
	newHeaders := make([]sarama.RecordHeader, len(headers), len(headers)+len(carrier))
	copy(newHeaders, headers)

	for k, v := range carrier {
		newHeaders = append(newHeaders, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	return newHeaders
}

// ExtractTraceContext extracts OpenTelemetry trace context from Kafka message headers
// for use in downstream consumers.
func ExtractTraceContext(ctx context.Context, headers []sarama.RecordHeader) context.Context {
	carrier := propagation.MapCarrier{}
	for _, h := range headers {
		carrier[string(h.Key)] = string(h.Value)
	}

	propagator := propagation.TraceContext{}
	return propagator.Extract(ctx, carrier)
}
