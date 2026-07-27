package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr                    string
	DatabaseMode                string // "turso" or "local" — required, no default
	DatabaseDSN                 string
	TursoDatabaseURL            string
	TursoAuthToken              string
	AdminAPIKey                 string
	MaxConcurrentReqs           int
	BuildNumber                 string
	CacheBuster                 string
	PublicBaseURL               string
	MailDomain                  string
	IMAPHost                    string
	IMAPPort                    int
	NotifierProvider            string // "unsend", "resend", "sendgrid", "mailgun", "log", or "" (deprecated cascade)
	SendGridAPIKey              string
	SendGridFromEmail           string
	SendGridFromName            string
	ResendAPIKey                string
	ResendFromEmail             string
	ResendFromName              string
	UnsendKey                   string
	UnsendBaseURL               string
	UnsendFromEmail             string
	UnsendFromName              string
	MailgunAPIKey               string
	MailgunDomain               string
	MailgunBaseURL              string
	MailgunFromEmail            string
	MailgunFromName             string
	PaddleAPIKey                string // required if Paddle is active provider
	PaddleWebhookSecret         string // optional
	PaddlePriceID               string // optional
	PaddleDefaultPaymentLinkURL string // optional
	PaddleClientToken           string // required if Paddle is active provider; must start with live_ or test_
	PaddleEnvironment           string // optional, defaults to "sandbox"; must be "sandbox" or "live"
	PaddleGiftDiscountID        string // optional
	PaddleGiftCouponCode        string // optional
	StripeSecretKey             string
	StripeWebhookSecret         string
	StripeSuccessURL            string
	StripeCancelURL             string
	StripeCurrency              string
	MailboxPriceCents           int64
	EdproofHMACSecret           string
	SupportEmail                string
}

func Load() (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	publicBaseURL := getEnv("PUBLIC_BASE_URL", "http://localhost:8080")

	dbMode := os.Getenv("DATABASE_MODE")
	if dbMode == "" {
		return nil, fmt.Errorf("DATABASE_MODE is required (set to \"turso\" or \"local\")")
	}
	if dbMode != "turso" && dbMode != "local" {
		return nil, fmt.Errorf("DATABASE_MODE must be \"turso\" or \"local\", got %q", dbMode)
	}
	if dbMode == "turso" {
		if os.Getenv("TURSO_DATABASE_URL") == "" {
			return nil, fmt.Errorf("DATABASE_MODE=turso requires TURSO_DATABASE_URL")
		}
		if os.Getenv("TURSO_AUTH_TOKEN") == "" {
			return nil, fmt.Errorf("DATABASE_MODE=turso requires TURSO_AUTH_TOKEN")
		}
	}

	notifierProvider := os.Getenv("NOTIFIER_PROVIDER")
	if err := validateNotifierProvider(notifierProvider); err != nil {
		return nil, err
	}

	edproofSecret := os.Getenv("EDPROOF_HMAC_SECRET")
	if edproofSecret == "" {
		return nil, fmt.Errorf("EDPROOF_HMAC_SECRET is required (generate with: openssl rand -hex 32)")
	}
	if len(edproofSecret) < 32 {
		return nil, fmt.Errorf("EDPROOF_HMAC_SECRET must be at least 32 bytes, got %d", len(edproofSecret))
	}

	paddleAPIKey := os.Getenv("PADDLE_API_KEY")
	paddleEnv := getEnv("PADDLE_ENVIRONMENT", "sandbox")
	paddleClientToken := os.Getenv("PADDLE_CLIENT_TOKEN")
	paddleWebhookSecret := os.Getenv("PADDLE_WEBHOOK_SECRET")
	paddlePriceID := os.Getenv("PADDLE_PRICE_ID")
	if err := validatePaddleConfig(paddleAPIKey, paddleEnv, paddleClientToken, paddlePriceID, paddleWebhookSecret); err != nil {
		return nil, err
	}

	return &Config{
		HTTPAddr:                    getEnv("HTTP_ADDR", ":8080"),
		DatabaseMode:                dbMode,
		DatabaseDSN:                 getEnv("DATABASE_DSN", "mailservice.db"),
		TursoDatabaseURL:            os.Getenv("TURSO_DATABASE_URL"),
		TursoAuthToken:              os.Getenv("TURSO_AUTH_TOKEN"),
		AdminAPIKey:                 os.Getenv("ADMIN_API_KEY"),
		MaxConcurrentReqs:           getEnvInt("MAX_CONCURRENT_REQUESTS", 100),
		BuildNumber:                 getEnv("BUILD_NUMBER", "dev"),
		CacheBuster:                 getEnv("CACHE_BUSTER", ""),
		PublicBaseURL:               publicBaseURL,
		MailDomain:                  getEnv("MAIL_DOMAIN", "mail.local"),
		IMAPHost:                    getEnv("IMAP_HOST", getEnv("MAIL_DOMAIN", "mail.local")),
		IMAPPort:                    getEnvInt("IMAP_PORT", 143),
		NotifierProvider:            notifierProvider,
		SendGridAPIKey:              os.Getenv("SENDGRID_API_KEY"),
		SendGridFromEmail:           getEnv("SENDGRID_FROM_EMAIL", ""),
		SendGridFromName:            getEnv("SENDGRID_FROM_NAME", "MailService"),
		ResendAPIKey:                os.Getenv("RESEND_API_KEY"),
		ResendFromEmail:             getEnv("RESEND_FROM_EMAIL", ""),
		ResendFromName:              getEnv("RESEND_FROM_NAME", "MailService"),
		UnsendKey:                   os.Getenv("UNSEND_KEY"),
		UnsendBaseURL:               getEnv("UNSEND_BASE_URL", "https://unsend.admin.lt/api"),
		UnsendFromEmail:             getEnv("UNSEND_FROM_EMAIL", ""),
		UnsendFromName:              getEnv("UNSEND_FROM_NAME", "MailService"),
		MailgunAPIKey:               os.Getenv("MAILGUN_API_KEY"),
		MailgunDomain:               os.Getenv("MAILGUN_DOMAIN"),
		MailgunBaseURL:              getEnv("MAILGUN_BASE_URL", "https://api.mailgun.net"),
		MailgunFromEmail:            getEnv("MAILGUN_FROM_EMAIL", ""),
		MailgunFromName:             getEnv("MAILGUN_FROM_NAME", "MailService"),
		PaddleAPIKey:                paddleAPIKey,
		PaddleWebhookSecret:         paddleWebhookSecret,
		PaddlePriceID:               paddlePriceID,
		PaddleDefaultPaymentLinkURL: getEnv("PADDLE_DEFAULT_PAYMENT_LINK_URL", ""),
		PaddleClientToken:           paddleClientToken,
		PaddleEnvironment:           paddleEnv,
		PaddleGiftDiscountID:        os.Getenv("PADDLE_GIFT_DISCOUNT_ID"),
		PaddleGiftCouponCode:        os.Getenv("PADDLE_GIFT_COUPON_CODE"),
		StripeSecretKey:             os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:         os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSuccessURL:            getEnv("STRIPE_SUCCESS_URL", "http://localhost:8080/payment/success"),
		StripeCancelURL:             getEnv("STRIPE_CANCEL_URL", "http://localhost:8080/payment/cancel"),
		StripeCurrency:              getEnv("STRIPE_CURRENCY", "usd"),
		MailboxPriceCents:           getEnvInt64("MAILBOX_PRICE_CENTS", 100),
		EdproofHMACSecret:           edproofSecret,
		SupportEmail:                getEnv("SUPPORT_EMAIL", "mbx_014d51a9d0b@truevipaccess.com"),
	}, nil
}

func loadDotEnv() error {
	values, err := godotenv.Read()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for key, value := range values {
		existing, exists := os.LookupEnv(key)
		if !exists || existing == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func getEnv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

var validNotifierProviders = map[string]bool{
	"unsend":   true,
	"resend":   true,
	"sendgrid": true,
	"mailgun":  true,
	"log":      true,
}

func validateNotifierProvider(provider string) error {
	if provider == "" {
		return nil // empty means deprecated cascade
	}
	if !validNotifierProviders[provider] {
		return fmt.Errorf("NOTIFIER_PROVIDER must be one of unsend, resend, sendgrid, mailgun, log — got %q", provider)
	}
	switch provider {
	case "unsend":
		if os.Getenv("UNSEND_KEY") == "" || os.Getenv("UNSEND_FROM_EMAIL") == "" {
			return fmt.Errorf("NOTIFIER_PROVIDER=unsend requires UNSEND_KEY and UNSEND_FROM_EMAIL")
		}
	case "resend":
		if os.Getenv("RESEND_API_KEY") == "" || os.Getenv("RESEND_FROM_EMAIL") == "" {
			return fmt.Errorf("NOTIFIER_PROVIDER=resend requires RESEND_API_KEY and RESEND_FROM_EMAIL")
		}
	case "sendgrid":
		if os.Getenv("SENDGRID_API_KEY") == "" || os.Getenv("SENDGRID_FROM_EMAIL") == "" {
			return fmt.Errorf("NOTIFIER_PROVIDER=sendgrid requires SENDGRID_API_KEY and SENDGRID_FROM_EMAIL")
		}
	case "mailgun":
		if os.Getenv("MAILGUN_API_KEY") == "" || os.Getenv("MAILGUN_DOMAIN") == "" || os.Getenv("MAILGUN_FROM_EMAIL") == "" {
			return fmt.Errorf("NOTIFIER_PROVIDER=mailgun requires MAILGUN_API_KEY, MAILGUN_DOMAIN, and MAILGUN_FROM_EMAIL")
		}
	}
	return nil
}

func validatePaddleConfig(apiKey, env, clientToken, priceID, webhookSecret string) error {
	// Only validate if PADDLE_API_KEY is set (indicating Paddle is configured).
	if apiKey == "" {
		return nil
	}

	if env != "sandbox" && env != "live" {
		return fmt.Errorf("PADDLE_ENVIRONMENT must be 'sandbox' or 'live', got %q", env)
	}

	// Validate API key prefix matches environment.
	expectedPrefix := "pdl_sdbx_apikey_"
	if env == "live" {
		expectedPrefix = "pdl_live_apikey_"
	}
	if !hasPrefix(apiKey, expectedPrefix) {
		return fmt.Errorf("PADDLE_API_KEY does not match PADDLE_ENVIRONMENT=%s: key must start with %q", env, expectedPrefix)
	}

	// PADDLE_CLIENT_TOKEN, PADDLE_PRICE_ID, and PADDLE_WEBHOOK_SECRET are
	// hard-required once Paddle is the active provider (PADDLE_API_KEY
	// set) — silently starting up without them defers the failure to the
	// first real checkout/webhook instead of catching it at startup, the
	// exact soft-fallthrough failure mode this migration was meant to
	// eliminate (see plan KTD-* / Important #1 of the final review).
	if clientToken == "" {
		return fmt.Errorf("PADDLE_CLIENT_TOKEN is required when PADDLE_API_KEY is set")
	}

	// Validate client token is not an API key.
	if hasPrefix(clientToken, "pdl_sdbx_apikey_") || hasPrefix(clientToken, "pdl_live_apikey_") {
		return fmt.Errorf("PADDLE_CLIENT_TOKEN must not be an API key (starting with pdl_*_apikey_); got credential shaped like an API key")
	}

	// Validate client token has correct shape (live_ or test_ prefix).
	if !hasPrefix(clientToken, "live_") && !hasPrefix(clientToken, "test_") {
		return fmt.Errorf("PADDLE_CLIENT_TOKEN must start with 'live_' or 'test_', got %q", clientToken)
	}

	if priceID == "" {
		return fmt.Errorf("PADDLE_PRICE_ID is required when PADDLE_API_KEY is set")
	}

	if webhookSecret == "" {
		return fmt.Errorf("PADDLE_WEBHOOK_SECRET is required when PADDLE_API_KEY is set")
	}

	return nil
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	if n < 0 {
		return fallback
	}
	return n
}
