package main

import (
	"log"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/config"
)

func TestSelectNotifierPrefersUnsendWhenConfigured(t *testing.T) {
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

func TestSelectNotifierFallsBackToResendWithoutUnsendFromEmail(t *testing.T) {
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
