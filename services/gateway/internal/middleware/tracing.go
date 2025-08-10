package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/samims/hcaas/pkg/tracing"
	"github.com/samims/hcaas/services/gateway/internal/logger"
)

// TracingMiddleware provides OpenTelemetry tracing for HTTP requests.
type TracingMiddleware struct {
	tracer *tracing.Tracer
	logger *logger.Logger
}

// NewTracingMiddleware creates a new tracing middleware.
func NewTracingMiddleware(logger *logger.Logger) *TracingMiddleware {
	// Get the global tracer
	otelTracer := otel.Tracer("hcaas-gateway")
	tracer := tracing.NewTracer(otelTracer)

	return &TracingMiddleware{
		tracer: tracer,
		logger: logger.WithComponent("tracing-middleware"),
	}
}

// Middleware returns the tracing middleware handler.
func (tm *TracingMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			
			// Start a new server span
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tm.tracer.StartServerSpan(ctx, spanName)
			defer span.End()

			// Add HTTP request attributes
			tm.tracer.AddRequestAttributes(span, r.Method, r.URL.Path, r.UserAgent(), 0)
			
			// Add additional attributes
			span.SetAttributes(
				attribute.String("http.scheme", getScheme(r)),
				attribute.String("http.host", r.Host),
				attribute.String("http.target", r.RequestURI),
				attribute.String("user_agent.original", r.UserAgent()),
			)

			// Create a response writer wrapper to capture status code
			wrappedWriter := &tracingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				span:          span,
				tracer:        tm.tracer,
			}

			// Update request context and process
			r = r.WithContext(ctx)
			next.ServeHTTP(wrappedWriter, r)

			// Update status code attribute after processing
			tm.tracer.AddAttributes(span, 
				attribute.Int("http.status_code", wrappedWriter.statusCode),
			)

			// Set span status based on HTTP status code
			if wrappedWriter.statusCode >= 400 {
				if wrappedWriter.statusCode >= 500 {
					span.SetStatus(codes.Error, "HTTP 5xx")
				} else {
					span.SetStatus(codes.Error, "HTTP 4xx")
				}
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// tracingResponseWriter wraps http.ResponseWriter to capture status code for tracing.
type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
	span       trace.Span
	tracer     *tracing.Tracer
}

func (trw *tracingResponseWriter) WriteHeader(code int) {
	if !trw.written {
		trw.statusCode = code
		trw.written = true
		trw.ResponseWriter.WriteHeader(code)
	}
}

func (trw *tracingResponseWriter) Write(data []byte) (int, error) {
	if !trw.written {
		trw.WriteHeader(http.StatusOK)
	}
	return trw.ResponseWriter.Write(data)
}

// getScheme determines the scheme (http/https) of the request.
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	
	// Check X-Forwarded-Proto header
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	
	return "http"
}
