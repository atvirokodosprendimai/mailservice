package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithGlobalSemaphoreRejectsWhenLimitReached(t *testing.T) {
	h := &Handler{concurrencySem: make(chan struct{}, 1)}
	h.concurrencySem <- struct{}{}

	handler := h.withGlobalSemaphore(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/mailboxes", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var payload struct {
		Error             string `json:"error"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RetryAfterSeconds < retryMinSeconds || payload.RetryAfterSeconds > retryMaxSeconds {
		t.Fatalf("retry_after_seconds out of range: %d", payload.RetryAfterSeconds)
	}
	if payload.Error == "" {
		t.Fatalf("expected non-empty error message")
	}
}
