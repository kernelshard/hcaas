// Package metrics provides Prometheus metrics for the API Gateway service.
//
// This package follows Google's SRE practices for monitoring and observability,
// implementing the four golden signals: latency, traffic, errors, and saturation.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Namespace for all API Gateway metrics
	namespace = "hcaas_gateway"
	
	// Common label names
	labelMethod     = "method"
	labelPath       = "path"
	labelStatusCode = "status_code"
	labelService    = "service"
	labelEndpoint   = "endpoint"
)

// Metrics holds all Prometheus metrics for the API Gateway.
type Metrics struct {
	// HTTP request metrics (Golden Signal: Traffic & Latency)
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	
	// Service proxy metrics
	upstreamRequestsTotal    *prometheus.CounterVec
	upstreamRequestDuration  *prometheus.HistogramVec
	upstreamRequestsInFlight *prometheus.GaugeVec
	
	// Rate limiting metrics
	rateLimitHits   *prometheus.CounterVec
	rateLimitActive *prometheus.GaugeVec
	
	// Authentication metrics
	authValidationsTotal *prometheus.CounterVec
	authValidationDuration *prometheus.HistogramVec
	
	// Error metrics (Golden Signal: Errors)
	errorTotal *prometheus.CounterVec
	
	// Resource utilization (Golden Signal: Saturation)
	activeConnections *prometheus.GaugeVec
	memoryUsage      *prometheus.GaugeVec
}

// New creates and registers all Prometheus metrics for the API Gateway.
// This follows Google's practice of registering metrics at startup.
func New() *Metrics {
	return &Metrics{
		// HTTP request metrics
		httpRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests processed by the gateway.",
			},
			[]string{labelMethod, labelPath, labelStatusCode},
		),
		
		httpRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "Duration of HTTP requests in seconds.",
				// Buckets optimized for API response times
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{labelMethod, labelPath, labelStatusCode},
		),
		
		// Upstream service metrics
		upstreamRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "upstream_requests_total",
				Help:      "Total number of requests sent to upstream services.",
			},
			[]string{labelService, labelMethod, labelEndpoint, labelStatusCode},
		),
		
		upstreamRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "upstream_request_duration_seconds",
				Help:      "Duration of upstream service requests in seconds.",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{labelService, labelMethod, labelEndpoint},
		),
		
		upstreamRequestsInFlight: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "upstream_requests_in_flight",
				Help:      "Number of upstream requests currently being processed.",
			},
			[]string{labelService},
		),
		
		// Rate limiting metrics
		rateLimitHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rate_limit_hits_total",
				Help:      "Total number of rate limit hits.",
			},
			[]string{"client_ip", "path"},
		),
		
		rateLimitActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rate_limit_active",
				Help:      "Number of active rate limit buckets.",
			},
			[]string{"path"},
		),
		
		// Authentication metrics
		authValidationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_validations_total",
				Help:      "Total number of JWT validations performed.",
			},
			[]string{"result"}, // success, failure, expired, invalid
		),
		
		authValidationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "auth_validation_duration_seconds",
				Help:      "Duration of JWT validation operations.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"result"},
		),
		
		// Error metrics
		errorTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total number of errors by type and component.",
			},
			[]string{"component", "error_type"},
		),
		
		// Resource utilization
		activeConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_connections",
				Help:      "Number of active connections to the gateway.",
			},
			[]string{"type"}, // http, websocket, etc.
		),
		
		memoryUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_usage_bytes",
				Help:      "Memory usage in bytes by component.",
			},
			[]string{"component"},
		),
	}
}

// RecordHTTPRequest records metrics for an HTTP request.
// This implements the Golden Signals for HTTP requests.
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)
	
	m.httpRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.httpRequestDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())
}

// RecordUpstreamRequest records metrics for upstream service requests.
func (m *Metrics) RecordUpstreamRequest(service, method, endpoint string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)
	
	m.upstreamRequestsTotal.WithLabelValues(service, method, endpoint, statusStr).Inc()
	m.upstreamRequestDuration.WithLabelValues(service, method, endpoint).Observe(duration.Seconds())
}

// RecordUpstreamRequestStart tracks the start of an upstream request.
func (m *Metrics) RecordUpstreamRequestStart(service string) {
	m.upstreamRequestsInFlight.WithLabelValues(service).Inc()
}

// RecordUpstreamRequestEnd tracks the end of an upstream request.
func (m *Metrics) RecordUpstreamRequestEnd(service string) {
	m.upstreamRequestsInFlight.WithLabelValues(service).Dec()
}

// RecordRateLimitHit records a rate limit hit.
func (m *Metrics) RecordRateLimitHit(clientIP, path string) {
	m.rateLimitHits.WithLabelValues(clientIP, path).Inc()
}

// SetRateLimitActive sets the number of active rate limit buckets for a path.
func (m *Metrics) SetRateLimitActive(path string, count float64) {
	m.rateLimitActive.WithLabelValues(path).Set(count)
}

// RecordAuthValidation records JWT validation metrics.
func (m *Metrics) RecordAuthValidation(result string, duration time.Duration) {
	m.authValidationsTotal.WithLabelValues(result).Inc()
	m.authValidationDuration.WithLabelValues(result).Observe(duration.Seconds())
}

// RecordError records an error by component and type.
func (m *Metrics) RecordError(component, errorType string) {
	m.errorTotal.WithLabelValues(component, errorType).Inc()
}

// SetActiveConnections sets the number of active connections.
func (m *Metrics) SetActiveConnections(connType string, count float64) {
	m.activeConnections.WithLabelValues(connType).Set(count)
}

// SetMemoryUsage sets memory usage for a component.
func (m *Metrics) SetMemoryUsage(component string, bytes float64) {
	m.memoryUsage.WithLabelValues(component).Set(bytes)
}

// AuthValidationResult constants for consistent labeling
const (
	AuthValidationSuccess = "success"
	AuthValidationFailure = "failure"
	AuthValidationExpired = "expired"
	AuthValidationInvalid = "invalid"
)

// ErrorType constants for consistent error categorization
const (
	ErrorTypeUpstream     = "upstream"
	ErrorTypeAuth         = "auth"
	ErrorTypeRateLimit    = "rate_limit"
	ErrorTypeValidation   = "validation"
	ErrorTypeInternal     = "internal"
)

// Component constants for consistent component identification
const (
	ComponentProxy        = "proxy"
	ComponentAuth         = "auth"
	ComponentRateLimit    = "rate_limit"
	ComponentMiddleware   = "middleware"
)
