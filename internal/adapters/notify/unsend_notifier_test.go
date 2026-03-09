package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnsendNotifierSendsPaymentLink(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAuth string
	var got map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"emailId":"eml_123"}`))
	}))
	defer ts.Close()

	n := NewUnsendNotifier(ts.URL, "test-key", "noreply@example.com", "MailService")
	err := n.SendPaymentLink(context.Background(), "user@example.com", "https://pay.example.com/session", "mbx-1")
	if err != nil {
		t.Fatalf("SendPaymentLink failed: %v", err)
	}

	if gotPath != "/v1/emails" {
		t.Fatalf("expected request path /v1/emails, got %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("expected bearer auth header, got %q", gotAuth)
	}
	if got["from"] != "MailService <noreply@example.com>" {
		t.Fatalf("unexpected from value: %#v", got["from"])
	}
	if got["subject"] == "" {
		t.Fatalf("expected subject to be set")
	}

	to, ok := got["to"].([]any)
	if !ok || len(to) != 1 || to[0] != "user@example.com" {
		t.Fatalf("unexpected to payload: %#v", got["to"])
	}

	text, _ := got["text"].(string)
	if !strings.Contains(text, "https://pay.example.com/session") {
		t.Fatalf("expected text payload to contain payment url, got %q", text)
	}
}

func TestUnsendNotifierReturnsErrorOnNon2xx(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer ts.Close()

	n := NewUnsendNotifier(ts.URL, "bad-key", "noreply@example.com", "MailService")
	err := n.SendRecoveryLink(context.Background(), "user@example.com", "https://example.com/recover")
	if err == nil {
		t.Fatal("expected send error")
	}
	if !strings.Contains(err.Error(), "unsend status 401") {
		t.Fatalf("unexpected error: %v", err)
	}
}
