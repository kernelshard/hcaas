package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	AttrGCPProjectID = "gcp.project_id"
	AttrGCPRegion    = "gcp.region"
	AttrGCPZone      = "gcp.zone"

	AttrServiceName        = "service.name"
	AttrServiceVersion     = "service.version"
	AttrServiceEnvironment = "service.environment"

	AttrHTTPMethod     = "http.method"
	AttrHTTPRoute      = "http.route"
	AttrHTTPUserAgent  = "http.user_agent"
	AttrHTTPStatusCode = "http.status_code"

	AttrDBOperation  = "db.operation"
	AttrDBTable      = "db.table"
	AttrDBDurationMs = "db.duration_ms"

	AttrMessagingSystem         = "messaging.system"
	AttrMessagingDestination    = "messaging.destination"
	AttrMessagingOperation      = "messaging.operation"
	AttrMessagingKafkaPartition = "messaging.kafka.partition"
	AttrMessagingKafkaOffset    = "messaging.kafka.offset"
)

// Tracer provides Google Cloud compliant tracing
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new tracer instance
func NewTracer(tracer trace.Tracer) *Tracer {
	return &Tracer{
		tracer: tracer,
	}
}

// StartServerSpan creates a new server span with Google Cloud attributes
func (t *Tracer) StartServerSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.startSpan(ctx, operation, trace.SpanKindServer, attrs...)
}

// StartClientSpan creates a new client span
func (t *Tracer) StartClientSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return t.startSpan(ctx, operation, trace.SpanKindClient, attrs...)
}

// startSpan is a helper to start a span with given kind and attributes
func (t *Tracer) startSpan(ctx context.Context, operation string, kind trace.SpanKind, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, operation,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(kind),
	)
	return ctx, span
}

// RecordError records an error on the span
func (t *Tracer) RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// AddAttributes adds attributes to span
func (t *Tracer) AddAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// AddGoogleCloudAttributes adds Google Cloud specific attributes
func (t *Tracer) AddGoogleCloudAttributes(span trace.Span, projectID, region, zone string) {
	span.SetAttributes(
		attribute.String(AttrGCPProjectID, projectID),
		attribute.String(AttrGCPRegion, region),
		attribute.String(AttrGCPZone, zone),
	)
}

// AddServiceAttributes adds service-specific attributes
func (t *Tracer) AddServiceAttributes(span trace.Span, serviceName, serviceVersion, environment string) {
	span.SetAttributes(
		attribute.String(AttrServiceName, serviceName),
		attribute.String(AttrServiceVersion, serviceVersion),
		attribute.String(AttrServiceEnvironment, environment),
	)
}

// AddRequestAttributes adds HTTP request attributes
func (t *Tracer) AddRequestAttributes(span trace.Span, method, path, userAgent string, statusCode int) {
	span.SetAttributes(
		attribute.String(AttrHTTPMethod, method),
		attribute.String(AttrHTTPRoute, path),
		attribute.String(AttrHTTPUserAgent, userAgent),
		attribute.Int(AttrHTTPStatusCode, statusCode),
	)
}

// AddDatabaseAttributes adds database operation attributes
func (t *Tracer) AddDatabaseAttributes(span trace.Span, operation, table string, duration time.Duration) {
	span.SetAttributes(
		attribute.String(AttrDBOperation, operation),
		attribute.String(AttrDBTable, table),
		attribute.Float64(AttrDBDurationMs, float64(duration.Milliseconds())),
	)
}

// AddKafkaAttributes adds Kafka operation attributes
func (t *Tracer) AddKafkaAttributes(span trace.Span, topic, operation string, partition int32, offset int64) {
	span.SetAttributes(
		attribute.String(AttrMessagingSystem, "kafka"),
		attribute.String(AttrMessagingDestination, topic),
		attribute.String(AttrMessagingOperation, operation),
		attribute.Int64(AttrMessagingKafkaPartition, int64(partition)),
		attribute.Int64(AttrMessagingKafkaOffset, offset),
	)
}

// GetTracer returns the global tracer
func GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
