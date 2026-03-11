package notify

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailgunSendPaymentLink(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAuth string
	var gotContentType string
	var gotBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"<msg-id>","message":"Queued."}`))
	}))
	defer ts.Close()

	n, err := NewMailgunNotifier("test-api-key", "mg.example.com", ts.URL, "noreply@example.com", "MailService")
	if err != nil {
		t.Fatalf("NewMailgunNotifier: %v", err)
	}

	err = n.SendPaymentLink(context.Background(), "user@example.com", "https://pay.example.com/session", "mbx-1")
	if err != nil {
		t.Fatalf("SendPaymentLink failed: %v", err)
	}

	if gotPath != "/v3/mg.example.com/messages" {
		t.Fatalf("expected path /v3/mg.example.com/messages, got %q", gotPath)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("api:test-api-key"))
	if gotAuth != expectedAuth {
		t.Fatalf("expected Basic Auth header %q, got %q", expectedAuth, gotAuth)
	}

	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("expected content-type application/x-www-form-urlencoded, got %q", gotContentType)
	}

	if !strings.Contains(gotBody, "from=MailService+%3Cnoreply%40example.com%3E") {
		t.Fatalf("expected from field with name, got body: %s", gotBody)
	}
	if !strings.Contains(gotBody, "to=user%40example.com") {
		t.Fatalf("expected to field, got body: %s", gotBody)
	}
	if !strings.Contains(gotBody, "subject=") {
		t.Fatalf("expected subject field, got body: %s", gotBody)
	}
	if !strings.Contains(gotBody, "html=") {
		t.Fatalf("expected html field, got body: %s", gotBody)
	}
}

func TestMailgunSendRecoveryLink(t *testing.T) {
	t.Parallel()

	var gotBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	n, err := NewMailgunNotifier("key", "mg.example.com", ts.URL, "noreply@example.com", "")
	if err != nil {
		t.Fatalf("NewMailgunNotifier: %v", err)
	}

	err = n.SendRecoveryLink(context.Background(), "user@example.com", "https://example.com/recover")
	if err != nil {
		t.Fatalf("SendRecoveryLink failed: %v", err)
	}

	if !strings.Contains(gotBody, "recover") {
		t.Fatalf("expected body to contain recovery URL, got: %s", gotBody)
	}
}

func TestMailgunNon2xxResponse(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
	}))
	defer ts.Close()

	n, err := NewMailgunNotifier("bad-key", "mg.example.com", ts.URL, "noreply@example.com", "MailService")
	if err != nil {
		t.Fatalf("NewMailgunNotifier: %v", err)
	}

	err = n.SendPaymentLink(context.Background(), "user@example.com", "https://pay.example.com", "mbx-1")
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "mailgun status 401") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Fatalf("expected error to contain response body, got: %v", err)
	}
}

func TestMailgunEUBaseURL(t *testing.T) {
	t.Parallel()

	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	n, err := NewMailgunNotifier("key", "mg.eu.example.com", ts.URL, "noreply@example.com", "")
	if err != nil {
		t.Fatalf("NewMailgunNotifier: %v", err)
	}

	err = n.SendPaymentLink(context.Background(), "user@example.com", "https://pay.example.com", "mbx-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/v3/mg.eu.example.com/messages" {
		t.Fatalf("expected EU domain in path, got %q", gotPath)
	}
}

func TestMailgunDomainValidation(t *testing.T) {
	t.Parallel()

	badDomains := []string{
		"../etc/passwd",
		"domain/with/slashes",
		"",
		"domain with spaces",
	}
	for _, d := range badDomains {
		_, err := NewMailgunNotifier("key", d, "", "noreply@example.com", "")
		if err == nil {
			t.Fatalf("expected error for domain %q", d)
		}
	}

	goodDomains := []string{
		"mg.example.com",
		"mail.example.co.uk",
		"sandbox123abc.mailgun.org",
	}
	for _, d := range goodDomains {
		_, err := NewMailgunNotifier("key", d, "", "noreply@example.com", "")
		if err != nil {
			t.Fatalf("unexpected error for domain %q: %v", d, err)
		}
	}
}

func TestMailgunFromFormatWithoutName(t *testing.T) {
	t.Parallel()

	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	n, err := NewMailgunNotifier("key", "mg.example.com", ts.URL, "noreply@example.com", "")
	if err != nil {
		t.Fatalf("NewMailgunNotifier: %v", err)
	}

	err = n.SendPaymentLink(context.Background(), "user@example.com", "https://pay.example.com", "mbx-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotBody, "from=noreply%40example.com") {
		t.Fatalf("expected bare email in from field (no name), got body: %s", gotBody)
	}
}
