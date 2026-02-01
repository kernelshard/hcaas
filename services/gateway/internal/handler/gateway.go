// Package handler provides HTTP handlers for the API Gateway service.
//
// This package implements request handling, response processing, and 
// integration with the service proxy following Google's API design patterns.
package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/samims/hcaas/services/gateway/internal/logger"
	"github.com/samims/hcaas/services/gateway/internal/metrics"
	"github.com/samims/hcaas/services/gateway/internal/proxy"
)

// GatewayHandler handles API Gateway requests and coordinates with the service proxy.
type GatewayHandler struct {
	proxy   *proxy.ServiceProxy
	logger  *logger.Logger
	metrics *metrics.Metrics
}

// NewGatewayHandler creates a new gateway handler.
func NewGatewayHandler(proxy *proxy.ServiceProxy, logger *logger.Logger, metrics *metrics.Metrics) *GatewayHandler {
	return &GatewayHandler{
		proxy:   proxy,
		logger:  logger.WithComponent("gateway-handler"),
		metrics: metrics,
	}
}

// ProxyHandler handles all proxy requests to upstream services.
func (h *GatewayHandler) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	
	// Proxy the request to the appropriate upstream service
	err := h.proxy.ProxyRequest(r.Context(), w, r)
	
	duration := time.Since(startTime)
	
	if err != nil {
		// Log the error
		h.logger.LogError(r.Context(), err, "Failed to proxy request",
			logger.FieldPath(r.URL.Path),
			logger.FieldMethod(r.Method),
			logger.FieldLatency(duration.Milliseconds()),
		)
		
		// Record error metrics
		h.metrics.RecordError(metrics.ComponentProxy, metrics.ErrorTypeUpstream)
		
		// Return error response
		h.handleProxyError(w, r, err)
		return
	}
	
	// Log successful proxy
	h.logger.WithContext(r.Context()).Debug("Request proxied successfully",
		logger.FieldPath(r.URL.Path),
		logger.FieldMethod(r.Method),
		logger.FieldLatency(duration.Milliseconds()),
	)
}

// handleProxyError handles proxy errors and returns appropriate HTTP responses.
func (h *GatewayHandler) handleProxyError(w http.ResponseWriter, r *http.Request, err error) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Content-Type", "application/json")
	
	// Determine status code based on error type
	statusCode := http.StatusInternalServerError
	message := "Internal server error"
	code := "PROXY_ERROR"
	
	// You might want to implement more sophisticated error classification
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "no route found"):
		statusCode = http.StatusNotFound
		message = "Endpoint not found"
		code = "ROUTE_NOT_FOUND"
	case strings.Contains(errStr, "timeout"):
		statusCode = http.StatusGatewayTimeout
		message = "Request timeout"
		code = "TIMEOUT_ERROR"
	case strings.Contains(errStr, "connection"):
		statusCode = http.StatusBadGateway
		message = "Upstream service unavailable"
		code = "SERVICE_UNAVAILABLE"
	}
	
	w.WriteHeader(statusCode)
	
	// Write JSON error response
	errorResponse := `{"error": "` + message + `", "code": "` + code + `"}`
	w.Write([]byte(errorResponse))
}
