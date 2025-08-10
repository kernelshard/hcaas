// Package proxy provides HTTP reverse proxy functionality for the API Gateway.
//
// This package implements intelligent request routing, load balancing,
// circuit breaking, and retry logic following Google's reliability patterns.
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/samims/hcaas/services/gateway/internal/config"
	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
)

// ServiceProxy handles proxying requests to upstream services.
type ServiceProxy struct {
	client  *http.Client
	config  *config.ServicesConfig
	logger  *logger.Logger
	metrics *metrics.Metrics
}

// NewServiceProxy creates a new service proxy with configured HTTP client.
func NewServiceProxy(cfg *config.ServicesConfig, logger *logger.Logger, metrics *metrics.Metrics) *ServiceProxy {
	// Configure HTTP client with timeouts and connection pooling
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		},
		// Default timeout - will be overridden per service
		Timeout: 30 * time.Second,
	}

	return &ServiceProxy{
		client:  client,
		config:  cfg,
		logger:  logger.WithComponent("proxy"),
		metrics: metrics,
	}
}

// ProxyRequest proxies an HTTP request to the appropriate upstream service.
func (p *ServiceProxy) ProxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	startTime := time.Now()

	// Determine target service based on request path
	serviceName, targetURL, err := p.determineTarget(r)
	if err != nil {
		p.metrics.RecordError(metrics.ComponentProxy, metrics.ErrorTypeValidation)
		return fmt.Errorf("failed to determine target: %w", err)
	}

	// Get service configuration
	serviceConfig, err := p.getServiceConfig(serviceName)
	if err != nil {
		// rather than client errors, so it's categorized differently for alerting
		p.metrics.RecordError(metrics.ComponentProxy, metrics.ErrorTypeInternal)
		return fmt.Errorf("failed to get service config: %w", err)
	}

	// OBSERVABILITY PHASE: Begin tracking this upstream request for monitoring
	// This creates a "request in flight" metric
	p.metrics.RecordUpstreamRequestStart(serviceName)

	// helps to identify the number of ongoing request
	defer p.metrics.RecordUpstreamRequestEnd(serviceName)

	// EXECUTION PHASE: Proxy the request with intelligent retry logic
	// This is where the actual network call happens, potentially with multiple attempts
	resp, err := p.proxyWithRetries(ctx, r, targetURL, serviceConfig)
	if err != nil {
		// record problems with the target service (network issues, service unavailable, timeouts, etc.)
		p.metrics.RecordError(metrics.ComponentProxy, metrics.ErrorTypeUpstream)
		return fmt.Errorf("failed to proxy request: %w", err)
	}

	// HTTP response bodies must be explicitly closed or they will leak memory and connections
	defer resp.Body.Close()

	// METRICS PHASE:  Calculate total request duration from start to finish
	duration := time.Since(startTime)
	p.metrics.RecordUpstreamRequest(serviceName, r.Method, r.URL.Path, resp.StatusCode, duration)

	// RESPONSE PHASE: Stream the upstream response back to the client
	// This copies status code, headers, and body from the upstream service response
	// to the client response, making the proxy transparent to the client
	if err := p.copyResponse(w, resp); err != nil {
		p.metrics.RecordError(metrics.ComponentProxy, metrics.ErrorTypeInternal)
		return fmt.Errorf("failed to copy response: %w", err)
	}

	// Log the service call
	p.logger.LogServiceCall(ctx, serviceName, r.Method, r.URL.Path, resp.StatusCode, int(duration.Milliseconds()))

	return nil
}

// determineTarget determines the target service and URL based on the request path.
func (p *ServiceProxy) determineTarget(r *http.Request) (string, string, error) {
	path := r.URL.Path

	switch {
	case strings.HasPrefix(path, "/auth/") || strings.HasPrefix(path, "/me"):
		// Route to auth service
		targetPath := strings.TrimPrefix(path, "/")
		targetURL := fmt.Sprintf("%s/%s", p.config.AuthService.BaseURL, targetPath)
		return "auth", targetURL, nil

	case strings.HasPrefix(path, "/urls"):
		// Route to URL service
		targetURL := fmt.Sprintf("%s%s", p.config.URLService.BaseURL, path)
		return "url", targetURL, nil

	case strings.HasPrefix(path, "/notifications"):
		// Route to notification service
		targetURL := fmt.Sprintf("%s%s", p.config.NotificationService.BaseURL, path)
		return "notification", targetURL, nil

	default:
		return "", "", fmt.Errorf("no route found for path: %s", path)
	}
}

// getServiceConfig returns the configuration for a specific service.
func (p *ServiceProxy) getServiceConfig(serviceName string) (*config.ServiceEndpoint, error) {
	switch serviceName {
	case "auth":
		return &p.config.AuthService, nil
	case "url":
		return &p.config.URLService, nil
	case "notification":
		return &p.config.NotificationService, nil
	default:
		return nil, fmt.Errorf("unknown service: %s", serviceName)
	}
}

// proxyWithRetries implements resilient communication patterns by automatically retrying failed
// requests according to the service configuration, using exponential backoff to avoid overwhelming
// failing services while maximizing the chance of eventual success.
func (p *ServiceProxy) proxyWithRetries(ctx context.Context, originalReq *http.Request, targetURL string, serviceConfig *config.ServiceEndpoint) (*http.Response, error) {
	// Track the last error encountered during retry attempts
	// This will be returned if all attempts fail
	var lastErr error

	for attempt := 0; attempt <= serviceConfig.Retries; attempt++ {
		// The createProxyRequest function handles body duplication and header copying
		req, err := p.createProxyRequest(ctx, originalReq, targetURL)

		if err != nil {
			// If we can't even create the request, fail immediately (no retry makes sense)
			return nil, fmt.Errorf("failed to create proxy request: %w", err)
		}

		// Apply per-request timeout to prevent individual requests from hanging indefinitely
		// Each attempt gets its own timeout context, independent of the overall operation timeout
		reqCtx, cancel := context.WithTimeout(ctx, serviceConfig.Timeout)
		req = req.WithContext(reqCtx)

		// Make the request
		resp, err := p.client.Do(req)
		cancel() // Always cancel the context to prevent resource leaks

		if err == nil {
			// Success - check if we should retry based on status code
			if !p.shouldRetry(resp.StatusCode) {
				return resp, nil
			}
			// Close the response body to free resources before retrying
			resp.Body.Close()
			lastErr = fmt.Errorf("upstream returned retryable status: %d", resp.StatusCode)
		} else {
			// Save this error in case all retries fail
			lastErr = err
		}

		// Implement exponential backoff between retry another attempts
		// Don't sleep after the final attempt since we're about to give up anyway

		if attempt < serviceConfig.Retries {
			// Calculate backoff duration: increases with each attempt to reduce load on failing services
			// Formula: (attempt + 1) * 100ms, so 100ms, 200ms, 300ms, etc.
			// The +1 ensures we don't have a 0ms delay on the first retry
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			// Use select statement to respect context cancellation during backoff
			select {
			case <-time.After(backoff):
				// Backoff period completed, proceed to next retry attempt
			case <-ctx.Done():
				// Parent context was cancelled (timeout, client disconnect, etc.)
				// Return immediately rather than continuing retry attempts
				return nil, ctx.Err()
			}
		}
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// createProxyRequest creates a new HTTP request for proxying.
func (p *ServiceProxy) createProxyRequest(ctx context.Context, originalReq *http.Request, targetURL string) (*http.Request, error) {
	// Parse target URL
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	// Handle request body preservation and duplication
	// HTTP request bodies are streams that can only be read once, so we need to:
	// 1. Read the entire body into memory
	// 2. Create a new reader for the proxy request
	// 3. Reset the original request body in case it needs to be read again (for retries)
	var body io.Reader
	if originalReq.Body != nil {
		// Read all bytes from the original request body
		bodyBytes, err := io.ReadAll(originalReq.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		// Create a new reader from the body bytes for the proxy request
		body = bytes.NewReader(bodyBytes)

		// Reset the original request body so it can be read again if needed
		// This is important for retry mechanisms or logging
		originalReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Create the new HTTP request that will be sent to the target service
	req, err := http.NewRequestWithContext(ctx, originalReq.Method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers (excluding hop-by-hop headers)
	for name, values := range originalReq.Header {
		// Only copy headers that are not hop-by-hop headers
		if !p.isHopByHopHeader(name) {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
	}

	// Add standard proxy forwarding headers to preserve client information
	// These headers help the target service understand the original client context

	// X-Forwarded-For: Contains the original client's IP address
	// This allows the target service to see the real client IP, not the proxy's IP
	req.Header.Set("X-Forwarded-For", p.getClientIP(originalReq))

	// X-Forwarded-Proto: Indicates the original protocol (http/https)
	req.Header.Set("X-Forwarded-Proto", p.getScheme(originalReq))

	// Helps target services understand what hostname the client originally requested
	req.Header.Set("X-Forwarded-Host", originalReq.Host)

	// Add gateway identification
	// Useful for troubleshooting, logging, and request tracing
	req.Header.Set("X-Gateway", "hcaas-gateway")

	return req, nil
}

// copyResponse copies the upstream response to the client.
func (p *ServiceProxy) copyResponse(w http.ResponseWriter, resp *http.Response) error {
	// Copy headers (excluding hop-by-hop headers)
	for name, values := range resp.Header {
		if !p.isHopByHopHeader(name) {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy body
	_, err := io.Copy(w, resp.Body)
	return err
}

// shouldRetry determines if a request should be retried based on status code.
func (p *ServiceProxy) shouldRetry(statusCode int) bool {
	// Retry on server errors and service unavailable
	return statusCode >= 500 && statusCode != 501 // Don't retry on "Not Implemented"
}

// isHopByHopHeader checks if a header is hop-by-hop and should not be forwarded.
func (p *ServiceProxy) isHopByHopHeader(name string) bool {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	name = strings.ToLower(name)
	for _, header := range hopByHopHeaders {
		if strings.ToLower(header) == name {
			return true
		}
	}
	return false
}

// getClientIP extracts the client IP address from the request.
func (p *ServiceProxy) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if parts := strings.Split(xff, ","); len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if host, _, found := strings.Cut(r.RemoteAddr, ":"); found {
		return host
	}

	return r.RemoteAddr
}

// getScheme determines the scheme (http/https) of the original request.
func (p *ServiceProxy) getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}

	// Check X-Forwarded-Proto header
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}

	return "http"
}
