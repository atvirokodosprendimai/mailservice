package main

import (
	"log"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/config"
)

// Deprecated cascade tests — these verify backward compatibility when NOTIFIER_PROVIDER is unset.

func TestSelectNotifierCascadePrefersUnsend(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		UnsendKey:         "unsend-key",
		UnsendBaseURL:     "https://unsend.admin.lt/api",
		UnsendFromEmail:   "unsend@example.com",
		UnsendFromName:    "MailService",
		ResendAPIKey:      "resend-key",
		ResendFromEmail:   "resend@example.com",
		SendGridAPIKey:    "sendgrid-key",
		SendGridFromEmail: "sendgrid@example.com",
	}

	n, provider := selectNotifier(cfg, log.Default())
	if n == nil {
		t.Fatal("expected notifier")
	}
	if provider != "unsend" {
		t.Fatalf("expected unsend provider, got %q", provider)
	}
}

func TestSelectNotifierCascadeFallsBackToResend(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		UnsendKey:       "unsend-key",
		UnsendBaseURL:   "https://unsend.admin.lt/api",
		ResendAPIKey:    "resend-key",
		ResendFromEmail: "resend@example.com",
	}

	_, provider := selectNotifier(cfg, log.Default())
	if provider != "resend" {
		t.Fatalf("expected resend provider, got %q", provider)
	}
}

// Explicit NOTIFIER_PROVIDER tests.

func TestSelectNotifierExplicitUnsend(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		NotifierProvider: "unsend",
		UnsendKey:        "unsend-key",
		UnsendBaseURL:    "https://unsend.admin.lt/api",
		UnsendFromEmail:  "unsend@example.com",
		UnsendFromName:   "MailService",
	}

	n, provider := selectNotifier(cfg, log.Default())
	if n == nil {
		t.Fatal("expected notifier")
	}
	if provider != "unsend" {
		t.Fatalf("expected unsend, got %q", provider)
	}
}

func TestSelectNotifierExplicitMailgun(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		NotifierProvider: "mailgun",
		MailgunAPIKey:    "test-key",
		MailgunDomain:    "mg.example.com",
		MailgunBaseURL:   "https://api.mailgun.net",
		MailgunFromEmail: "noreply@example.com",
		MailgunFromName:  "MailService",
	}

	n, provider := selectNotifier(cfg, log.Default())
	if n == nil {
		t.Fatal("expected notifier")
	}
	if provider != "mailgun" {
		t.Fatalf("expected mailgun, got %q", provider)
	}
}

func TestSelectNotifierExplicitLog(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		NotifierProvider: "log",
	}

	n, provider := selectNotifier(cfg, log.Default())
	if n == nil {
		t.Fatal("expected notifier")
	}
	if provider != "log" {
		t.Fatalf("expected log, got %q", provider)
	}
}

func TestSelectNotifierExplicitIgnoresCascade(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		NotifierProvider:  "resend",
		ResendAPIKey:      "resend-key",
		ResendFromEmail:   "resend@example.com",
		UnsendKey:         "unsend-key",
		UnsendFromEmail:   "unsend@example.com",
		UnsendBaseURL:     "https://unsend.admin.lt/api",
		SendGridAPIKey:    "sg-key",
		SendGridFromEmail: "sg@example.com",
	}

	_, provider := selectNotifier(cfg, log.Default())
	if provider != "resend" {
		t.Fatalf("expected resend (explicit), got %q", provider)
	}
}
