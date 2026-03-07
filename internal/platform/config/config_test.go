package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	dotEnv := []byte("HTTP_ADDR=:9090\nSTRIPE_CURRENCY=eur\n")
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
}
