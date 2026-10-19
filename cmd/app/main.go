package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atvirokodosprendimai/mailservice/docs"
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
	"github.com/atvirokodosprendimai/mailservice/internal/platform/imapstats"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	metricsRegistry := metrics.NewRegistry(ctx)
	journalUnit := os.Getenv("IMAP_LOG_JOURNAL_UNIT")
	shipper := imapstats.NewShipper(metricsRegistry, journalUnit, log.Default())
	go func() {
		if err := shipper.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("imapstats shipper: %v", err)
		}
	}()

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

	openCollective, err := payment.NewOpenCollectiveGateway(payment.OpenCollectiveConfig{
		CheckoutURL:   cfg.OpenCollectiveCheckoutURL,
		APIURL:        cfg.OpenCollectiveAPIURL,
		PersonalToken: cfg.OpenCollectivePersonalToken,
		ProjectSlug:   cfg.OpenCollectiveProjectSlug,
		TierSlug:      cfg.OpenCollectiveTierSlug,
	})
	if err != nil {
		log.Fatalf("Open Collective configuration: %v", err)
	}
	var paymentGateway ports.PaymentGateway = openCollective
	mailboxService := service.NewMailboxService(mailboxRepo, accountRepo, paymentGateway, notifier, tokenGen, mailRuntimeProvisioner, imapReader, cfg.MailDomain, cfg.IMAPHost, cfg.IMAPPort)
	mailboxService.SetMetrics(metricsRegistry)
	mailboxService.SetFreeMode(cfg.FreeMode)
	mailboxService.SetPublicBaseURL(cfg.PublicBaseURL)
	supportMessageRepo := repository.NewSupportMessageRepository(db)
	mailboxService.SetSupportConfig(service.SupportConfig{
		SupportEmail: cfg.SupportEmail,
		SupportRepo:  supportMessageRepo,
	})
	log.Printf("support contact enabled (email: %s)", cfg.SupportEmail)
	accountService := service.NewAccountService(accountRepo, accountRecoveryRepo, refreshTokenRepo, notifier, tokenGen, cfg.PublicBaseURL)

	log.Printf("edproof challenge-response enabled")

	handler := httpapi.NewHandler(httpapi.Config{
		AdminAPIKey:         cfg.AdminAPIKey,
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		MaxConcurrentReqs:   cfg.MaxConcurrentReqs,
		BuildNumber:         cfg.BuildNumber,
		CacheBuster:         cfg.CacheBuster,
		KeyProofVerifier:    newKeyProofVerifier(),
		PaymentGateway:      paymentGateway,
		MailboxService:      mailboxService,
		AccountService:      accountService,
		Logger:              log.Default(),
		ChallengeAuth:       edproof.NewAuthenticator([]byte(cfg.EdproofHMACSecret)),
		AgentAPISkillDoc:    docs.AgentAPISkill,
		MockPaymentMode:     false,
		Metrics:             metricsRegistry,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHTTPMiddleware(handler.Routes(), metricsRegistry),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("mail service listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// Background sweep: expire mailboxes whose ExpiresAt has passed.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := mailboxService.ExpireMailboxes(ctx)
				if err != nil {
					log.Printf("expiry sweep error: %v", err)
				} else if n > 0 {
					log.Printf("expiry sweep: expired %d mailbox(es)", n)
				}
			}
		}
	}()

	if cfg.OpenCollectivePersonalToken == "" || cfg.OpenCollectiveProjectSlug == "" || cfg.OpenCollectiveTierSlug == "" {
		log.Printf("Open Collective reconciliation off: personal token, project slug or tier slug is unset")
	} else if !cfg.FreeMode {
		go func() {
			ticker := time.NewTicker(cfg.OpenCollectiveReconcileInterval)
			defer ticker.Stop()
			reconcile := func() {
				n, err := mailboxService.ReconcileOpenCollectivePayments(ctx, openCollective)
				if err != nil {
					log.Printf("Open Collective reconciliation: %v", err)
				} else if n > 0 {
					log.Printf("Open Collective reconciliation: applied %d charge(s)", n)
				}
			}
			reconcile()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					reconcile()
				}
			}
		}()
	}

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
