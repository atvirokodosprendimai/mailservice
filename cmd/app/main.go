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

	db, err := database.OpenAndMigrate(cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("database init: %v", err)
	}

	mailboxRepo := repository.NewMailboxRepository(db)
	mailRuntimeProvisioner := repository.NewMailRuntimeProvisioner(db, cfg.MailDomain)
	accountRepo := repository.NewAccountRepository(db)
	accountRecoveryRepo := repository.NewAccountRecoveryRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	tokenGen := token.NewSecureGenerator()

	var notifier ports.Notifier = notify.NewLogNotifier(log.Default())
	if cfg.ResendAPIKey != "" && cfg.ResendFromEmail != "" {
		notifier = notify.NewResendNotifier(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.ResendFromName)
		log.Printf("resend notifier enabled")
	} else if cfg.SendGridAPIKey != "" && cfg.SendGridFromEmail != "" {
		notifier = notify.NewSendGridNotifier(cfg.SendGridAPIKey, cfg.SendGridFromEmail, cfg.SendGridFromName)
		log.Printf("sendgrid notifier enabled")
	} else {
		log.Printf("email providers disabled, using log notifier")
	}

	var paymentGateway ports.PaymentGateway = payment.NewMockGateway(cfg.PublicBaseURL)
	if cfg.StripeSecretKey != "" {
		paymentGateway = payment.NewStripeGateway(payment.StripeConfig{
			SecretKey:  cfg.StripeSecretKey,
			PriceCents: cfg.MailboxPriceCents,
			Currency:   cfg.StripeCurrency,
			SuccessURL: cfg.StripeSuccessURL,
			CancelURL:  cfg.StripeCancelURL,
		})
		log.Printf("stripe enabled")
	} else {
		log.Printf("stripe disabled, using mock payment links")
	}

	mailboxService := service.NewMailboxService(mailboxRepo, accountRepo, paymentGateway, notifier, tokenGen, mailRuntimeProvisioner)
	accountService := service.NewAccountService(accountRepo, accountRecoveryRepo, refreshTokenRepo, notifier, tokenGen, cfg.PublicBaseURL)

	handler := httpapi.NewHandler(httpapi.Config{
		StripeWebhookSecret: cfg.StripeWebhookSecret,
		MaxConcurrentReqs:   cfg.MaxConcurrentReqs,
		MailboxService:      mailboxService,
		AccountService:      accountService,
		Logger:              log.Default(),
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
