package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	dotEnv := []byte("DATABASE_MODE=local\nHTTP_ADDR=:9090\nSTRIPE_CURRENCY=eur\nMAX_CONCURRENT_REQUESTS=77\nMAIL_DOMAIN=mx.example.com\nIMAP_HOST=imap.example.com\nIMAP_PORT=1143\nSENDGRID_FROM_EMAIL=noreply@example.com\nRESEND_FROM_EMAIL=hello@example.com\nUNSEND_BASE_URL=https://unsend.admin.lt/api\nUNSEND_FROM_EMAIL=mail@example.com\n")
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

	t.Setenv("DATABASE_MODE", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("STRIPE_CURRENCY", "")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "")
	t.Setenv("MAIL_DOMAIN", "")
	t.Setenv("IMAP_HOST", "")
	t.Setenv("IMAP_PORT", "")
	t.Setenv("SENDGRID_FROM_EMAIL", "")
	t.Setenv("RESEND_FROM_EMAIL", "")
	t.Setenv("UNSEND_BASE_URL", "")
	t.Setenv("UNSEND_FROM_EMAIL", "")
	t.Setenv("UNSEND_KEY", "")
	t.Setenv("EDPROOF_HMAC_SECRET", "0123456789abcdef0123456789abcdef")

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
	if cfg.UnsendBaseURL != "https://unsend.admin.lt/api" {
		t.Fatalf("expected unsend base url from .env, got %q", cfg.UnsendBaseURL)
	}
	if cfg.UnsendKey != "" {
		t.Fatalf("expected empty unsend key by default, got %q", cfg.UnsendKey)
	}
	if cfg.UnsendFromEmail != "mail@example.com" {
		t.Fatalf("expected unsend from email from .env, got %q", cfg.UnsendFromEmail)
	}
}

func TestPaddleConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		env           string
		clientToken   string
		priceID       string
		webhookSecret string
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "happy path sandbox",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "test_client123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       false,
		},
		{
			name:          "happy path live",
			apiKey:        "pdl_live_apikey_test123",
			env:           "live",
			clientToken:   "live_client123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       false,
		},
		{
			name:    "no api key set (paddle not configured)",
			apiKey:  "",
			env:     "sandbox",
			wantErr: false,
		},
		{
			name:          "invalid environment",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "invalid",
			clientToken:   "test_client123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_ENVIRONMENT must be 'sandbox' or 'live'",
		},
		{
			name:          "sandbox key with live environment",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "live",
			clientToken:   "test_client123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "does not match PADDLE_ENVIRONMENT",
		},
		{
			name:          "live key with sandbox environment",
			apiKey:        "pdl_live_apikey_test123",
			env:           "sandbox",
			clientToken:   "test_client123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "does not match PADDLE_ENVIRONMENT",
		},
		{
			name:          "client token missing",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN is required",
		},
		{
			name:          "client token is api key",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "pdl_sdbx_apikey_wrong",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN must not be an API key",
		},
		{
			name:          "client token is live api key",
			apiKey:        "pdl_live_apikey_test123",
			env:           "live",
			clientToken:   "pdl_live_apikey_wrong",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN must not be an API key",
		},
		{
			name:          "client token has invalid shape",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "invalid_token_123",
			priceID:       "pri_test123",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN must start with 'live_' or 'test_'",
		},
		{
			name:          "price id missing",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "test_client123",
			priceID:       "",
			webhookSecret: "whsec_test123",
			wantErr:       true,
			wantErrSubstr: "PADDLE_PRICE_ID is required",
		},
		{
			name:          "webhook secret missing",
			apiKey:        "pdl_sdbx_apikey_test123",
			env:           "sandbox",
			clientToken:   "test_client123",
			priceID:       "pri_test123",
			webhookSecret: "",
			wantErr:       true,
			wantErrSubstr: "PADDLE_WEBHOOK_SECRET is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePaddleConfig(tt.apiKey, tt.env, tt.clientToken, tt.priceID, tt.webhookSecret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePaddleConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Fatalf("validatePaddleConfig() error = %v, want substring %q", err, tt.wantErrSubstr)
			}
		})
	}
}

func TestLoadPaddleConfig(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		wantErr       bool
		wantErrSubstr string
		checkFn       func(*testing.T, *Config)
	}{
		{
			name: "load with all paddle vars set",
			env: map[string]string{
				"DATABASE_MODE":                   "local",
				"EDPROOF_HMAC_SECRET":             "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":                  "pdl_sdbx_apikey_test123",
				"PADDLE_ENVIRONMENT":              "sandbox",
				"PADDLE_CLIENT_TOKEN":             "test_client123",
				"PADDLE_WEBHOOK_SECRET":           "whsec_test123",
				"PADDLE_PRICE_ID":                 "pri_test123",
				"PADDLE_DEFAULT_PAYMENT_LINK_URL": "https://paddle.example.com",
				"PADDLE_GIFT_DISCOUNT_ID":         "dis_test123",
				"PADDLE_GIFT_COUPON_CODE":         "gift_test",
			},
			wantErr: false,
			checkFn: func(t *testing.T, cfg *Config) {
				if cfg.PaddleAPIKey != "pdl_sdbx_apikey_test123" {
					t.Errorf("expected PaddleAPIKey, got %q", cfg.PaddleAPIKey)
				}
				if cfg.PaddleEnvironment != "sandbox" {
					t.Errorf("expected PaddleEnvironment=sandbox, got %q", cfg.PaddleEnvironment)
				}
				if cfg.PaddleClientToken != "test_client123" {
					t.Errorf("expected PaddleClientToken, got %q", cfg.PaddleClientToken)
				}
				if cfg.PaddleWebhookSecret != "whsec_test123" {
					t.Errorf("expected PaddleWebhookSecret, got %q", cfg.PaddleWebhookSecret)
				}
				if cfg.PaddlePriceID != "pri_test123" {
					t.Errorf("expected PaddlePriceID, got %q", cfg.PaddlePriceID)
				}
				if cfg.PaddleDefaultPaymentLinkURL != "https://paddle.example.com" {
					t.Errorf("expected PaddleDefaultPaymentLinkURL, got %q", cfg.PaddleDefaultPaymentLinkURL)
				}
				if cfg.PaddleGiftDiscountID != "dis_test123" {
					t.Errorf("expected PaddleGiftDiscountID, got %q", cfg.PaddleGiftDiscountID)
				}
				if cfg.PaddleGiftCouponCode != "gift_test" {
					t.Errorf("expected PaddleGiftCouponCode, got %q", cfg.PaddleGiftCouponCode)
				}
			},
		},
		{
			name: "load with sandbox defaults",
			env: map[string]string{
				"DATABASE_MODE":         "local",
				"EDPROOF_HMAC_SECRET":   "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":        "pdl_sdbx_apikey_test123",
				"PADDLE_CLIENT_TOKEN":   "test_client123",
				"PADDLE_PRICE_ID":       "pri_test123",
				"PADDLE_WEBHOOK_SECRET": "whsec_test123",
			},
			wantErr: false,
			checkFn: func(t *testing.T, cfg *Config) {
				if cfg.PaddleEnvironment != "sandbox" {
					t.Errorf("expected default PaddleEnvironment=sandbox, got %q", cfg.PaddleEnvironment)
				}
			},
		},
		{
			name: "paddle active but client token missing",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_sdbx_apikey_test123",
			},
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN is required",
		},
		{
			name: "paddle active but price id missing",
			env: map[string]string{
				"DATABASE_MODE":         "local",
				"EDPROOF_HMAC_SECRET":   "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":        "pdl_sdbx_apikey_test123",
				"PADDLE_CLIENT_TOKEN":   "test_client123",
				"PADDLE_WEBHOOK_SECRET": "whsec_test123",
			},
			wantErr:       true,
			wantErrSubstr: "PADDLE_PRICE_ID is required",
		},
		{
			name: "paddle active but webhook secret missing",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_sdbx_apikey_test123",
				"PADDLE_CLIENT_TOKEN": "test_client123",
				"PADDLE_PRICE_ID":     "pri_test123",
			},
			wantErr:       true,
			wantErrSubstr: "PADDLE_WEBHOOK_SECRET is required",
		},
		{
			name: "api key mismatch sandbox key with live env",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_sdbx_apikey_test123",
				"PADDLE_ENVIRONMENT":  "live",
			},
			wantErr:       true,
			wantErrSubstr: "does not match PADDLE_ENVIRONMENT",
		},
		{
			name: "api key mismatch live key with sandbox env",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_live_apikey_test123",
				"PADDLE_ENVIRONMENT":  "sandbox",
			},
			wantErr:       true,
			wantErrSubstr: "does not match PADDLE_ENVIRONMENT",
		},
		{
			name: "client token is api key shape",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_sdbx_apikey_test123",
				"PADDLE_ENVIRONMENT":  "sandbox",
				"PADDLE_CLIENT_TOKEN": "pdl_sdbx_apikey_wrong",
			},
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN must not be an API key",
		},
		{
			name: "client token has invalid shape",
			env: map[string]string{
				"DATABASE_MODE":       "local",
				"EDPROOF_HMAC_SECRET": "0123456789abcdef0123456789abcdef",
				"PADDLE_API_KEY":      "pdl_sdbx_apikey_test123",
				"PADDLE_ENVIRONMENT":  "sandbox",
				"PADDLE_CLIENT_TOKEN": "invalid_token_123",
			},
			wantErr:       true,
			wantErrSubstr: "PADDLE_CLIENT_TOKEN must start with 'live_' or 'test_'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Paddle env vars first
			os.Unsetenv("PADDLE_API_KEY")
			os.Unsetenv("PADDLE_ENVIRONMENT")
			os.Unsetenv("PADDLE_CLIENT_TOKEN")
			os.Unsetenv("PADDLE_WEBHOOK_SECRET")
			os.Unsetenv("PADDLE_PRICE_ID")
			os.Unsetenv("PADDLE_DEFAULT_PAYMENT_LINK_URL")
			os.Unsetenv("PADDLE_GIFT_DISCOUNT_ID")
			os.Unsetenv("PADDLE_GIFT_COUPON_CODE")

			// Set provided env vars
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
				t.Fatalf("Load() error = %v, want substring %q", err, tt.wantErrSubstr)
			}
			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, cfg)
			}
		})
	}
}
