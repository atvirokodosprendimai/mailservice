package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr            string
	DatabaseDSN         string
	MaxConcurrentReqs   int
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
	PolarToken          string
	PolarServerURL      string
	PolarPriceID        string
	PolarSuccessURL     string
	PolarReturnURL      string
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

	return &Config{
		HTTPAddr:            getEnv("HTTP_ADDR", ":8080"),
		DatabaseDSN:         getEnv("DATABASE_DSN", "mailservice.db"),
		MaxConcurrentReqs:   getEnvInt("MAX_CONCURRENT_REQUESTS", 100),
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
		PolarToken:          os.Getenv("POLAR_TOKEN"),
		PolarServerURL:      getEnv("POLAR_SERVER_URL", "https://api.polar.sh"),
		PolarPriceID:        getEnv("POLAR_PRICE_ID", ""),
		PolarSuccessURL:     polarSuccessURL,
		PolarReturnURL:      polarReturnURL,
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
