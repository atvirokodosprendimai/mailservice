package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr                        string
	DatabaseMode                    string // "turso" or "local" — required, no default
	DatabaseDSN                     string
	TursoDatabaseURL                string
	TursoAuthToken                  string
	AdminAPIKey                     string
	MaxConcurrentReqs               int
	BuildNumber                     string
	CacheBuster                     string
	PublicBaseURL                   string
	MailDomain                      string
	IMAPHost                        string
	IMAPPort                        int
	FreeMode                        bool   // default off; set FREE_MODE=true for the free activation flow
	NotifierProvider                string // "unsend", "resend", "sendgrid", "mailgun", "log", or "" (deprecated cascade)
	SendGridAPIKey                  string
	SendGridFromEmail               string
	SendGridFromName                string
	ResendAPIKey                    string
	ResendFromEmail                 string
	ResendFromName                  string
	UnsendKey                       string
	UnsendBaseURL                   string
	UnsendFromEmail                 string
	UnsendFromName                  string
	MailgunAPIKey                   string
	MailgunDomain                   string
	MailgunBaseURL                  string
	MailgunFromEmail                string
	MailgunFromName                 string
	OpenCollectiveCheckoutURL       string
	OpenCollectivePersonalToken     string
	OpenCollectiveProjectSlug       string
	OpenCollectiveTierSlug          string
	OpenCollectiveAPIURL            string
	OpenCollectiveReconcileInterval time.Duration
	StripeSecretKey                 string
	StripeWebhookSecret             string
	StripeSuccessURL                string
	StripeCancelURL                 string
	StripeCurrency                  string
	MailboxPriceCents               int64
	EdproofHMACSecret               string
	SupportEmail                    string
}

func Load() (*Config, error) {
	if err := loadDotEnv(); err != nil {
		return nil, err
	}

	publicBaseURL := getEnv("PUBLIC_BASE_URL", "http://localhost:8080")
	interval, err := time.ParseDuration(getEnv("OPENCOLLECTIVE_RECONCILE_INTERVAL", "15m"))
	if err != nil || interval <= 0 {
		return nil, fmt.Errorf("OPENCOLLECTIVE_RECONCILE_INTERVAL must be a positive duration")
	}

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

	return &Config{
		HTTPAddr:                        getEnv("HTTP_ADDR", ":8080"),
		DatabaseMode:                    dbMode,
		DatabaseDSN:                     getEnv("DATABASE_DSN", "mailservice.db"),
		TursoDatabaseURL:                os.Getenv("TURSO_DATABASE_URL"),
		TursoAuthToken:                  os.Getenv("TURSO_AUTH_TOKEN"),
		AdminAPIKey:                     os.Getenv("ADMIN_API_KEY"),
		MaxConcurrentReqs:               getEnvInt("MAX_CONCURRENT_REQUESTS", 100),
		BuildNumber:                     getEnv("BUILD_NUMBER", "dev"),
		CacheBuster:                     getEnv("CACHE_BUSTER", ""),
		PublicBaseURL:                   publicBaseURL,
		MailDomain:                      getEnv("MAIL_DOMAIN", "mail.local"),
		IMAPHost:                        getEnv("IMAP_HOST", getEnv("MAIL_DOMAIN", "mail.local")),
		IMAPPort:                        getEnvInt("IMAP_PORT", 143),
		FreeMode:                        getEnvBool("FREE_MODE", false),
		NotifierProvider:                notifierProvider,
		SendGridAPIKey:                  os.Getenv("SENDGRID_API_KEY"),
		SendGridFromEmail:               getEnv("SENDGRID_FROM_EMAIL", ""),
		SendGridFromName:                getEnv("SENDGRID_FROM_NAME", "MailService"),
		ResendAPIKey:                    os.Getenv("RESEND_API_KEY"),
		ResendFromEmail:                 getEnv("RESEND_FROM_EMAIL", ""),
		ResendFromName:                  getEnv("RESEND_FROM_NAME", "MailService"),
		UnsendKey:                       os.Getenv("UNSEND_KEY"),
		UnsendBaseURL:                   getEnv("UNSEND_BASE_URL", "https://unsend.admin.lt/api"),
		UnsendFromEmail:                 getEnv("UNSEND_FROM_EMAIL", ""),
		UnsendFromName:                  getEnv("UNSEND_FROM_NAME", "MailService"),
		MailgunAPIKey:                   os.Getenv("MAILGUN_API_KEY"),
		MailgunDomain:                   os.Getenv("MAILGUN_DOMAIN"),
		MailgunBaseURL:                  getEnv("MAILGUN_BASE_URL", "https://api.mailgun.net"),
		MailgunFromEmail:                getEnv("MAILGUN_FROM_EMAIL", ""),
		MailgunFromName:                 getEnv("MAILGUN_FROM_NAME", "MailService"),
		OpenCollectiveCheckoutURL:       getEnv("OPENCOLLECTIVE_CHECKOUT_URL", "https://opencollective.com/it-uoga/projects/truevipaccess/contribute/mailbox-monthly-106782/checkout"),
		OpenCollectivePersonalToken:     os.Getenv("OPENCOLLECTIVE_PERSONAL_TOKEN"),
		OpenCollectiveProjectSlug:       getEnv("OPENCOLLECTIVE_PROJECT_SLUG", "truevipaccess"),
		OpenCollectiveTierSlug:          getEnv("OPENCOLLECTIVE_TIER_SLUG", "mailbox-monthly"),
		OpenCollectiveAPIURL:            getEnv("OPENCOLLECTIVE_API_URL", "https://api.opencollective.com/graphql/v2"),
		OpenCollectiveReconcileInterval: interval,
		StripeSecretKey:                 os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:             os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripeSuccessURL:                getEnv("STRIPE_SUCCESS_URL", "http://localhost:8080/payment/success"),
		StripeCancelURL:                 getEnv("STRIPE_CANCEL_URL", "http://localhost:8080/payment/cancel"),
		StripeCurrency:                  getEnv("STRIPE_CURRENCY", "usd"),
		MailboxPriceCents:               getEnvInt64("MAILBOX_PRICE_CENTS", 100),
		EdproofHMACSecret:               edproofSecret,
		SupportEmail:                    getEnv("SUPPORT_EMAIL", "mbx_014d51a9d0b@truevipaccess.com"),
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

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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
