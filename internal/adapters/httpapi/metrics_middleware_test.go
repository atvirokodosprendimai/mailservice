package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func TestHTTPMiddlewareRecordsDuration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := metrics.NewRegistry(ctx)

	handler := NewHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}), registry)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if got := registry.Histogram("http_latency_ms").Percentile(0.50); got <= 0 {
		t.Fatalf("latency p50 = %d, want > 0", got)
	}
}

func TestHTTPMiddlewareCaptures5xxResponseBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry := metrics.NewRegistry(ctx)

	handler := NewHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "database exploded", http.StatusInternalServerError)
	}), registry)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	handler.ServeHTTP(rec, req)

	entries := registry.TopN("top_errors").Snapshot()
	if len(entries) != 1 {
		t.Fatalf("top_errors length = %d, want 1", len(entries))
	}
	if !strings.Contains(entries[0].Msg, "database exploded") {
		t.Fatalf("top error message = %q, want database exploded", entries[0].Msg)
	}
	if entries[0].Count != 1 {
		t.Fatalf("top error count = %d, want 1", entries[0].Count)
	}
}
