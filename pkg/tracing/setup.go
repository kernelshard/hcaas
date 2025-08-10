package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// SetupTracing initializes OpenTelemetry with Google Cloud best practices
func SetupTracing(ctx context.Context, logger *slog.Logger) (func(context.Context) error, error) {
	config := NewConfig()

	// Create gRPC exporter
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpointURL(config.OTLPExporterEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create resource with Google Cloud attributes
	resourceAttrs := []attribute.KeyValue{
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.DeploymentEnvironment(config.Environment),
		attribute.String("cloud.provider", "gcp"),
		attribute.String("gcp.project_id", config.GCPProjectID),
		attribute.String("service.namespace", "hcaas"),
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(resourceAttrs...),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(1.0),
		)),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for distributed tracing
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp.Shutdown, nil
}
