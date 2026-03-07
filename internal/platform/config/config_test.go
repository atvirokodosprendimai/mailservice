package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	dotEnv := []byte("HTTP_ADDR=:9090\nSTRIPE_CURRENCY=eur\nMAX_CONCURRENT_REQUESTS=77\nMAIL_DOMAIN=mx.example.com\nIMAP_HOST=imap.example.com\nIMAP_PORT=1143\nSENDGRID_FROM_EMAIL=noreply@example.com\nRESEND_FROM_EMAIL=hello@example.com\n")
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), dotEnv, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}

	t.Setenv("HTTP_ADDR", "")
	t.Setenv("STRIPE_CURRENCY", "")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "")
	t.Setenv("MAIL_DOMAIN", "")
	t.Setenv("IMAP_HOST", "")
	t.Setenv("IMAP_PORT", "")
	t.Setenv("SENDGRID_FROM_EMAIL", "")
	t.Setenv("RESEND_FROM_EMAIL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("expected HTTP addr from .env, got %q", cfg.HTTPAddr)
	}
	if cfg.StripeCurrency != "eur" {
		t.Fatalf("expected Stripe currency from .env, got %q", cfg.StripeCurrency)
	}
	if cfg.MaxConcurrentReqs != 77 {
		t.Fatalf("expected max concurrent requests from .env, got %d", cfg.MaxConcurrentReqs)
	}
	if cfg.MailDomain != "mx.example.com" {
		t.Fatalf("expected mail domain from .env, got %q", cfg.MailDomain)
	}
	if cfg.IMAPHost != "imap.example.com" {
		t.Fatalf("expected imap host from .env, got %q", cfg.IMAPHost)
	}
	if cfg.IMAPPort != 1143 {
		t.Fatalf("expected imap port from .env, got %d", cfg.IMAPPort)
	}
	if cfg.SendGridFromEmail != "noreply@example.com" {
		t.Fatalf("expected sendgrid from email from .env, got %q", cfg.SendGridFromEmail)
	}
	if cfg.ResendFromEmail != "hello@example.com" {
		t.Fatalf("expected resend from email from .env, got %q", cfg.ResendFromEmail)
	}
}
