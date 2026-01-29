package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/kernelshard/hcaas/services/url/internal/metrics"
)

func MetricsMiddleware(next http.Handler) http.Handler {
	h := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)
		duration := time.Since(start).Seconds()
		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(ww.Status())

		metrics.RequestCount.WithLabelValues(path, method, status).Inc()
		metrics.RequestDuration.WithLabelValues(path, method).Observe(duration)
	}

	return http.HandlerFunc(h)
}
