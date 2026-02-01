// Package logger provides structured logging functionality for the API Gateway service.
//
// This package follows Google's logging practices with structured JSON logging,
// context-aware logging, and proper log levels.
package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
}

// New creates a new structured logger instance.
// It uses JSON handler for production environments and text handler for development.
func New(serviceName string) *Logger {
	// Determine log level from environment
	level := getLogLevel()
	
	// Create handler options
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug, // Only add source in debug mode
	}

	// Use JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stdout, opts)
	
	// Create logger with service name as a constant field
	logger := slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("version", getVersion()),
	)

	return &Logger{Logger: logger}
}

// WithContext returns a logger with context-derived fields.
// This is useful for adding request-scoped information like trace IDs.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l.Logger
	
	// Add trace ID if available in context
	if traceID := getTraceIDFromContext(ctx); traceID != "" {
		logger = logger.With(slog.String("trace_id", traceID))
	}
	
	// Add span ID if available in context
	if spanID := getSpanIDFromContext(ctx); spanID != "" {
		logger = logger.With(slog.String("span_id", spanID))
	}
	
	return &Logger{Logger: logger}
}

// WithFields returns a logger with additional structured fields.
// This follows Google's practice of adding structured context to logs.
func (l *Logger) WithFields(fields ...slog.Attr) *Logger {
	args := make([]any, 0, len(fields))
	for _, field := range fields {
		args = append(args, field)
	}
	return &Logger{Logger: l.With(args...)}
}

// WithComponent returns a logger with a component field.
// This is useful for distinguishing between different parts of the service.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{Logger: l.With(slog.String("component", component))}
}

// LogRequest logs HTTP request information in a structured format.
func (l *Logger) LogRequest(ctx context.Context, method, path, userAgent string, statusCode, duration int) {
	l.WithContext(ctx).Info("HTTP request",
		slog.String("method", method),
		slog.String("path", path),
		slog.String("user_agent", userAgent),
		slog.Int("status_code", statusCode),
		slog.Int("duration_ms", duration),
	)
}

// LogError logs error information with additional context.
func (l *Logger) LogError(ctx context.Context, err error, msg string, fields ...slog.Attr) {
	logger := l.WithContext(ctx)
	
	args := []any{
		slog.String("error", err.Error()),
	}
	
	for _, field := range fields {
		args = append(args, field)
	}
	
	logger.Error(msg, args...)
}

// LogServiceCall logs outbound service calls for observability.
func (l *Logger) LogServiceCall(ctx context.Context, service, method, endpoint string, statusCode, duration int) {
	l.WithContext(ctx).Info("Service call",
		slog.String("target_service", service),
		slog.String("method", method),
		slog.String("endpoint", endpoint),
		slog.Int("status_code", statusCode),
		slog.Int("duration_ms", duration),
	)
}

// Helper functions

func getLogLevel() slog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "DEBUG", "debug":
		return slog.LevelDebug
	case "INFO", "info":
		return slog.LevelInfo
	case "WARN", "warn", "WARNING", "warning":
		return slog.LevelWarn
	case "ERROR", "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getVersion() string {
	// In a real application, this might come from build flags or version file
	return os.Getenv("SERVICE_VERSION")
}

// getTraceIDFromContext extracts trace ID from OpenTelemetry context.
func getTraceIDFromContext(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		return spanContext.TraceID().String()
	}
	return ""
}

// getSpanIDFromContext extracts span ID from OpenTelemetry context.
func getSpanIDFromContext(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		return spanContext.SpanID().String()
	}
	return ""
}

// Common log fields for consistent logging
var (
	FieldUserID    = func(userID string) slog.Attr { return slog.String("user_id", userID) }
	FieldRequestID = func(requestID string) slog.Attr { return slog.String("request_id", requestID) }
	FieldLatency   = func(ms int64) slog.Attr { return slog.Int64("latency_ms", ms) }
	FieldStatus    = func(status int) slog.Attr { return slog.Int("status", status) }
	FieldMethod    = func(method string) slog.Attr { return slog.String("method", method) }
	FieldPath      = func(path string) slog.Attr { return slog.String("path", path) }
	FieldService   = func(service string) slog.Attr { return slog.String("service", service) }
)
