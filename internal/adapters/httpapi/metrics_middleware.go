package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func NewHTTPMiddleware(next http.Handler, reg *metrics.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &metricsResponseWriter{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		reg.Histogram("http_latency_ms").Observe(time.Since(start).Milliseconds())
		if recorder.statusCode >= http.StatusInternalServerError && recorder.body.Len() > 0 {
			reg.TopN("top_errors").Inc(recorder.body.String())
		}
	})
}

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *metricsResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	if w.body.Len() < 200 {
		remaining := 200 - w.body.Len()
		if len(data) < remaining {
			remaining = len(data)
		}
		w.body.Write(data[:remaining])
	}
	return w.ResponseWriter.Write(data)
}
