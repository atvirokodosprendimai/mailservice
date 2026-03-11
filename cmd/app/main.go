package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/adapters/httpapi"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/identity/edproof"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/imap"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/notify"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/payment"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/repository"
	"github.com/atvirokodosprendimai/mailservice/internal/adapters/token"
	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/config"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Local SQLite — always needed for Postfix/Dovecot mail_users and mail_domains tables
	localDB, err := database.OpenAndMigrate(cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("local database init: %v", err)
	}

	// App database — explicit mode selection, no silent fallback
	var db = localDB
	if cfg.DatabaseMode == "turso" {
		tursoDB, err := database.OpenTurso(cfg.TursoDatabaseURL, cfg.TursoAuthToken)
		if err != nil {
			log.Fatalf("turso database init: %v", err)
		}
		db = tursoDB
		log.Printf("database mode: turso (%s)", cfg.TursoDatabaseURL)
	} else {
		log.Printf("database mode: local (%s)", cfg.DatabaseDSN)
	}

	mailboxRepo := repository.NewMailboxRepository(db)
	mailRuntimeProvisioner := repository.NewMailRuntimeProvisioner(localDB, cfg.MailDomain)
	imapReader := imap.NewReader()
	accountRepo := repository.NewAccountRepository(db)
	accountRecoveryRepo := repository.NewAccountRecoveryRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	tokenGen := token.NewSecureGenerator()

	notifier, notifierProvider := selectNotifier(cfg, log.Default())
	log.Printf("%s notifier enabled", notifierProvider)

	var paymentGateway ports.PaymentGateway = payment.NewMockGateway(cfg.PublicBaseURL)
	if cfg.PolarToken != "" && cfg.PolarProductID != "" {
		paymentGateway = payment.NewPolarGateway(payment.PolarConfig{
			ServerURL:  cfg.PolarServerURL,
			Token:      cfg.PolarToken,
			ProductID:  cfg.PolarProductID,
			SuccessURL: cfg.PolarSuccessURL,
			ReturnURL:  cfg.PolarReturnURL,
		})
		log.Printf("polar enabled")
	} else if cfg.PolarToken != "" || cfg.PolarProductID != "" {
		log.Printf("polar partially configured, falling back to legacy payment provider selection")
	} else if cfg.StripeSecretKey != "" {
		paymentGateway = payment.NewStripeGateway(payment.StripeConfig{
			SecretKey:  cfg.StripeSecretKey,
			PriceCents: cfg.MailboxPriceCents,
			Currency:   cfg.StripeCurrency,
			SuccessURL: cfg.StripeSuccessURL,
			CancelURL:  cfg.StripeCancelURL,
		})
		log.Printf("stripe enabled")
	} else {
		log.Printf("real payment providers disabled, using mock payment links")
	}

	mailboxService := service.NewMailboxService(mailboxRepo, accountRepo, paymentGateway, notifier, tokenGen, mailRuntimeProvisioner, imapReader, cfg.MailDomain, cfg.IMAPHost, cfg.IMAPPort)
	accountService := service.NewAccountService(accountRepo, accountRecoveryRepo, refreshTokenRepo, notifier, tokenGen, cfg.PublicBaseURL)

	log.Printf("edproof challenge-response enabled")

	handler := httpapi.NewHandler(httpapi.Config{
		AdminAPIKey:         cfg.AdminAPIKey,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		PolarWebhookSecret:  cfg.PolarWebhookSecret,
		MaxConcurrentReqs:   cfg.MaxConcurrentReqs,
		BuildNumber:         cfg.BuildNumber,
		CacheBuster:         cfg.CacheBuster,
		KeyProofVerifier:    newKeyProofVerifier(),
		PaymentGateway:      paymentGateway,
		MailboxService:      mailboxService,
		AccountService:      accountService,
		Logger:              log.Default(),
		EdproofHMACSecret:   []byte(cfg.EdproofHMACSecret),
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("mail service listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown: %v", err)
	}
}

func newKeyProofVerifier() ports.KeyProofVerifier {
	return edproof.NewVerifier(nil)
}

func selectNotifier(cfg *config.Config, logger *log.Logger) (ports.Notifier, string) {
	if cfg.NotifierProvider != "" {
		return selectNotifierExplicit(cfg, logger)
	}

	logger.Printf("DEPRECATED: implicit notifier cascade will be removed in a future version. Set NOTIFIER_PROVIDER explicitly.")
	return selectNotifierCascade(cfg, logger)
}

func selectNotifierExplicit(cfg *config.Config, logger *log.Logger) (ports.Notifier, string) {
	switch cfg.NotifierProvider {
	case "unsend":
		return notify.NewUnsendNotifier(cfg.UnsendBaseURL, cfg.UnsendKey, cfg.UnsendFromEmail, cfg.UnsendFromName), "unsend"
	case "resend":
		return notify.NewResendNotifier(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.ResendFromName), "resend"
	case "sendgrid":
		return notify.NewSendGridNotifier(cfg.SendGridAPIKey, cfg.SendGridFromEmail, cfg.SendGridFromName), "sendgrid"
	case "mailgun":
		n, err := notify.NewMailgunNotifier(cfg.MailgunAPIKey, cfg.MailgunDomain, cfg.MailgunBaseURL, cfg.MailgunFromEmail, cfg.MailgunFromName)
		if err != nil {
			log.Fatalf("mailgun notifier: %v", err)
		}
		return n, "mailgun"
	case "log":
		return notify.NewLogNotifier(logger), "log"
	default:
		log.Fatalf("unknown NOTIFIER_PROVIDER %q", cfg.NotifierProvider)
		return nil, ""
	}
}

func selectNotifierCascade(cfg *config.Config, logger *log.Logger) (ports.Notifier, string) {
	if cfg.UnsendKey != "" && cfg.UnsendFromEmail != "" {
		return notify.NewUnsendNotifier(cfg.UnsendBaseURL, cfg.UnsendKey, cfg.UnsendFromEmail, cfg.UnsendFromName), "unsend"
	}
	if cfg.ResendAPIKey != "" && cfg.ResendFromEmail != "" {
		return notify.NewResendNotifier(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.ResendFromName), "resend"
	}
	if cfg.SendGridAPIKey != "" && cfg.SendGridFromEmail != "" {
		return notify.NewSendGridNotifier(cfg.SendGridAPIKey, cfg.SendGridFromEmail, cfg.SendGridFromName), "sendgrid"
	}
	return notify.NewLogNotifier(logger), "log"
}
