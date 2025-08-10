package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracerInterface defines the methods for tracing
type TracerInterface interface {
	StartSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span)
	StartClientSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span)
	RecordError(span trace.Span, err error)
	AddAttributes(span trace.Span, attrs ...attribute.KeyValue)
	AddGoogleCloudAttributes(span trace.Span, projectID, region, zone string)
	AddServiceAttributes(span trace.Span, serviceName, serviceVersion, environment string)
	AddRequestAttributes(span trace.Span, method, path, userAgent string, statusCode int)
	AddDatabaseAttributes(span trace.Span, operation, table string, duration float64)
	AddKafkaAttributes(span trace.Span, topic, operation string, partition int32, offset int64)
}

// ConfigInterface defines the methods for configuration
type ConfigInterface interface {
	Validate() error
}
