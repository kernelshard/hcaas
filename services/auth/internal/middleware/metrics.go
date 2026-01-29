package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/kernelshard/hcaas/services/auth/internal/metrics"
)

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		duration := time.Since(startTime).Seconds()
		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(ww.Status())

		// inject values from every request
		metrics.TotalRequests.WithLabelValues("hcaas_auth", method, path, status).Inc()
		metrics.RequestDuration.WithLabelValues("hcaas_auth", method, path, status).Observe(duration)
	})
}
