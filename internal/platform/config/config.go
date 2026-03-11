package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr            string
	DatabaseMode        string // "turso" or "local" — required, no default
	DatabaseDSN         string
	TursoDatabaseURL    string
	TursoAuthToken      string
	MaxConcurrentReqs   int
	BuildNumber         string
	CacheBuster         string
	PublicBaseURL       string
	MailDomain          string
	IMAPHost            string
	IMAPPort            int
	SendGridAPIKey      string
	SendGridFromEmail   string
	SendGridFromName    string
	ResendAPIKey        string
	ResendFromEmail     string
	ResendFromName      string
	UnsendKey           string
	UnsendBaseURL       string
	UnsendFromEmail     string
	UnsendFromName      string
	PolarToken          string
	PolarServerURL      string
	PolarProductID      string
	PolarSuccessURL     string
	PolarReturnURL      string
	PolarWebhookSecret  string
	StripeSecretKey     string
	StripeWebhookSecret string
	StripeSuccessURL    string
	StripeCancelURL     string
	StripeCurrency      string
	MailboxPriceCents   int64
}

func Load() (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	publicBaseURL := getEnv("PUBLIC_BASE_URL", "http://localhost:8080")
	polarSuccessURL := getEnv("POLAR_SUCCESS_URL", publicBaseURL+"/v1/payments/polar/success?checkout_id={CHECKOUT_ID}")
	polarReturnURL := getEnv("POLAR_RETURN_URL", publicBaseURL)

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

	return &Config{
		HTTPAddr:            getEnv("HTTP_ADDR", ":8080"),
		DatabaseMode:        dbMode,
		DatabaseDSN:         getEnv("DATABASE_DSN", "mailservice.db"),
		TursoDatabaseURL:    os.Getenv("TURSO_DATABASE_URL"),
		TursoAuthToken:      os.Getenv("TURSO_AUTH_TOKEN"),
		MaxConcurrentReqs:   getEnvInt("MAX_CONCURRENT_REQUESTS", 100),
		BuildNumber:         getEnv("BUILD_NUMBER", "dev"),
		CacheBuster:         getEnv("CACHE_BUSTER", ""),
		PublicBaseURL:       publicBaseURL,
		MailDomain:          getEnv("MAIL_DOMAIN", "mail.local"),
		IMAPHost:            getEnv("IMAP_HOST", getEnv("MAIL_DOMAIN", "mail.local")),
		IMAPPort:            getEnvInt("IMAP_PORT", 143),
		SendGridAPIKey:      os.Getenv("SENDGRID_API_KEY"),
		SendGridFromEmail:   getEnv("SENDGRID_FROM_EMAIL", ""),
		SendGridFromName:    getEnv("SENDGRID_FROM_NAME", "MailService"),
		ResendAPIKey:        os.Getenv("RESEND_API_KEY"),
		ResendFromEmail:     getEnv("RESEND_FROM_EMAIL", ""),
		ResendFromName:      getEnv("RESEND_FROM_NAME", "MailService"),
		UnsendKey:           os.Getenv("UNSEND_KEY"),
		UnsendBaseURL:       getEnv("UNSEND_BASE_URL", "https://unsend.admin.lt/api"),
		UnsendFromEmail:     getEnv("UNSEND_FROM_EMAIL", ""),
		UnsendFromName:      getEnv("UNSEND_FROM_NAME", "MailService"),
		PolarToken:          os.Getenv("POLAR_TOKEN"),
		PolarServerURL:      getEnv("POLAR_SERVER_URL", "https://api.polar.sh"),
		PolarProductID:      getEnv("POLAR_PRODUCT_ID", getEnv("POLAR_PRICE_ID", "")),
		PolarSuccessURL:     polarSuccessURL,
		PolarReturnURL:      polarReturnURL,
		PolarWebhookSecret:  os.Getenv("POLAR_WEBHOOK_SECRET"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSuccessURL:    getEnv("STRIPE_SUCCESS_URL", "http://localhost:8080/payment/success"),
		StripeCancelURL:     getEnv("STRIPE_CANCEL_URL", "http://localhost:8080/payment/cancel"),
		StripeCurrency:      getEnv("STRIPE_CURRENCY", "usd"),
		MailboxPriceCents:   getEnvInt64("MAILBOX_PRICE_CENTS", 299),
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
