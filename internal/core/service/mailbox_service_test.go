package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

func TestCreateMailboxReturnsExistingPendingMailbox(t *testing.T) {
	repo := &fakeMailboxRepo{
		pendingByAccount: map[string]*domain.Mailbox{
			"acc-1": {
				ID:         "mbx-1",
				AccountID:  "acc-1",
				Status:     domain.MailboxStatusPendingPayment,
				PaymentURL: "http://pay/1",
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	provisioner := &fakeMailRuntimeProvisioner{}
	accounts := &fakeMailboxAccountRepo{}
	service := NewMailboxService(repo, accounts, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com"},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected pending mailbox reuse, got created=true")
	}
	if mailbox.ID != "mbx-1" {
		t.Fatalf("expected existing mailbox id, got %q", mailbox.ID)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no notifier call, got %d", notifier.calls)
	}
}

func TestClaimMailboxRefreshesPaymentForExistingUnpaidKey(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				BillingEmail:   "billing@example.com",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusPendingPayment,
				PaymentURL:     "http://pay/1",
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.ClaimMailbox(context.Background(), "renewed@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.ID != "mbx-1" {
		t.Fatalf("expected existing mailbox id, got %q", mailbox.ID)
	}
	if mailbox.BillingEmail != "renewed@example.com" {
		t.Fatalf("expected billing email refresh, got %q", mailbox.BillingEmail)
	}
	if payment.calls != 1 {
		t.Fatalf("expected payment link refresh, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected notifier call, got %d", notifier.calls)
	}
	if repo.updated == nil || repo.updated.PaymentSessionID == "" {
		t.Fatalf("expected mailbox update with payment session")
	}
}

func TestClaimMailboxReusesExistingPendingPaymentSession(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-reuse": {
				ID:               "mbx-reuse",
				BillingEmail:     "billing@example.com",
				KeyFingerprint:   "edproof:key-reuse",
				Status:           domain.MailboxStatusPendingPayment,
				PaymentSessionID: "existing-session-123",
				PaymentURL:       "https://checkout.polar.sh/existing",
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-reuse",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.PaymentSessionID != "existing-session-123" {
		t.Fatalf("expected existing session ID preserved, got %q", mailbox.PaymentSessionID)
	}
	if mailbox.PaymentURL != "https://checkout.polar.sh/existing" {
		t.Fatalf("expected existing payment URL preserved, got %q", mailbox.PaymentURL)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no new payment link creation, got %d calls", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no notifier call, got %d calls", notifier.calls)
	}
}

func TestClaimMailboxValidatesExistingPendingPaymentSession(t *testing.T) {
	transientErr := errors.New("polar unavailable")

	tests := []struct {
		name              string
		getPaymentSession func(context.Context, string) (*ports.PaymentSession, error)
		wantSessionID     string
		wantPaymentURL    string
		wantCreateCalls   int
		wantNotifierCalls int
		wantErr           error
	}{
		{
			name: "open session is reused",
			getPaymentSession: func(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
				return &ports.PaymentSession{
					SessionID: sessionID,
					Status:    ports.PaymentSessionStatusOpen,
				}, nil
			},
			wantSessionID:   "existing-session-123",
			wantPaymentURL:  "https://checkout.polar.sh/existing",
			wantCreateCalls: 0,
		},
		{
			name: "missing session regenerates",
			getPaymentSession: func(_ context.Context, _ string) (*ports.PaymentSession, error) {
				return nil, ports.ErrPaymentSessionNotFound
			},
			wantSessionID:     "sess-1",
			wantPaymentURL:    "http://pay/1",
			wantCreateCalls:   1,
			wantNotifierCalls: 1,
		},
		{
			name: "expired session regenerates",
			getPaymentSession: func(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
				return &ports.PaymentSession{
					SessionID: sessionID,
					Status:    ports.PaymentSessionStatusExpired,
				}, nil
			},
			wantSessionID:     "sess-1",
			wantPaymentURL:    "http://pay/1",
			wantCreateCalls:   1,
			wantNotifierCalls: 1,
		},
		{
			name: "failed session regenerates",
			getPaymentSession: func(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
				return &ports.PaymentSession{
					SessionID: sessionID,
					Status:    ports.PaymentSessionStatusFailed,
				}, nil
			},
			wantSessionID:     "sess-1",
			wantPaymentURL:    "http://pay/1",
			wantCreateCalls:   1,
			wantNotifierCalls: 1,
		},
		{
			name: "transient error returns wrapped error",
			getPaymentSession: func(_ context.Context, _ string) (*ports.PaymentSession, error) {
				return nil, transientErr
			},
			wantErr: transientErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeMailboxRepo{
				byKeyFingerprint: map[string]*domain.Mailbox{
					"edproof:key-reuse": {
						ID:               "mbx-reuse",
						BillingEmail:     "billing@example.com",
						KeyFingerprint:   "edproof:key-reuse",
						Status:           domain.MailboxStatusPendingPayment,
						PaymentSessionID: "existing-session-123",
						PaymentURL:       "https://checkout.polar.sh/existing",
					},
				},
			}
			payment := &fakePaymentGateway{getPaymentSession: tt.getPaymentSession}
			notifier := &fakeMailboxNotifier{}
			service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

			mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
				Fingerprint: "edproof:key-reuse",
				Algorithm:   "ed25519",
			}, "")
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected wrapped %v, got %v", tt.wantErr, err)
				}
				if payment.calls != 0 {
					t.Fatalf("expected no new payment link creation, got %d calls", payment.calls)
				}
				if notifier.calls != 0 {
					t.Fatalf("expected no notifier call, got %d calls", notifier.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("ClaimMailbox failed: %v", err)
			}
			if created {
				t.Fatalf("expected existing mailbox reuse, got created=true")
			}
			if payment.getCalls != 1 {
				t.Fatalf("expected one payment session lookup, got %d", payment.getCalls)
			}
			if mailbox.PaymentSessionID != tt.wantSessionID {
				t.Fatalf("expected session ID %q, got %q", tt.wantSessionID, mailbox.PaymentSessionID)
			}
			if mailbox.PaymentURL != tt.wantPaymentURL {
				t.Fatalf("expected payment URL %q, got %q", tt.wantPaymentURL, mailbox.PaymentURL)
			}
			if payment.calls != tt.wantCreateCalls {
				t.Fatalf("expected %d payment link creations, got %d", tt.wantCreateCalls, payment.calls)
			}
			if notifier.calls != tt.wantNotifierCalls {
				t.Fatalf("expected %d notifier calls, got %d", tt.wantNotifierCalls, notifier.calls)
			}
		})
	}
}

func TestClaimMailboxCreatesPendingMailboxForNewKey(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-2",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new mailbox to be created")
	}
	if mailbox.BillingEmail != "billing@example.com" {
		t.Fatalf("expected billing email, got %q", mailbox.BillingEmail)
	}
	if mailbox.KeyFingerprint != "edproof:key-2" {
		t.Fatalf("expected key fingerprint, got %q", mailbox.KeyFingerprint)
	}
	if mailbox.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected pending status, got %s", mailbox.Status)
	}
	if payment.calls != 1 {
		t.Fatalf("expected one payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one notifier call, got %d", notifier.calls)
	}
}

func TestClaimMailboxAllowsSameEmailForSameKey(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-same": {
				ID:             "mbx-same",
				BillingEmail:   "same@example.com",
				KeyFingerprint: "edproof:key-same",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC()),
				ExpiresAt:      &future,
			},
		},
		activeOrPendingByBillingEmail: map[string]*domain.Mailbox{
			"same@example.com": {
				ID:             "mbx-same",
				BillingEmail:   "same@example.com",
				KeyFingerprint: "edproof:key-same",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC()),
				ExpiresAt:      &future,
			},
		},
	}
	payment := &fakePaymentGateway{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := svc.ClaimMailbox(context.Background(), "same@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-same",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox should succeed for same key, got %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.ID != "mbx-same" {
		t.Fatalf("expected existing mailbox, got %q", mailbox.ID)
	}
}

func TestClaimMailboxReturnsExistingActiveMailboxWithoutRefreshingPayment(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-3": {
				ID:             "mbx-3",
				BillingEmail:   "billing@example.com",
				KeyFingerprint: "edproof:key-3",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:      &future,
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-3",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.ID != "mbx-3" {
		t.Fatalf("expected existing mailbox id, got %q", mailbox.ID)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link refresh, got %d", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no notifier call, got %d", notifier.calls)
	}
}

func TestNewMailboxServiceDefaultsIMAPHostToMailDomain(t *testing.T) {
	t.Parallel()

	service := NewMailboxService(
		&fakeMailboxRepo{},
		&fakeMailboxAccountRepo{},
		&fakePaymentGateway{},
		&fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"},
		&fakeMailRuntimeProvisioner{},
		&fakeMailReader{},
		" MX.Example.com ",
		"  ",
		143,
	)

	if service.imapHost != "mx.example.com" {
		t.Fatalf("expected imapHost to default to normalized mailDomain, got %q", service.imapHost)
	}
}

func TestCreateMailboxActiveSubscriptionSkipsPaymentAndProvisioned(t *testing.T) {
	now := time.Now().UTC().Add(24 * time.Hour)
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	provisioner := &fakeMailRuntimeProvisioner{}
	accounts := &fakeMailboxAccountRepo{}
	service := NewMailboxService(repo, accounts, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com", SubscriptionExpiresAt: &now},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if !created {
		t.Fatalf("expected mailbox to be newly created")
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active mailbox for subscribed account, got %s", mailbox.Status)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no payment notification, got %d", notifier.calls)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected one runtime provision, got %d", provisioner.calls)
	}
}

func TestMarkMailboxPaidEnsuresRuntimeMailbox(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-1": {
				ID:               "mbx-1",
				AccountID:        "acc-1",
				IMAPUsername:     "mbx_abc",
				IMAPPassword:     "pass",
				PaymentSessionID: "sess-1",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acc-1": {ID: "acc-1"},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, err := service.MarkMailboxPaid(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", mailbox.Status)
	}
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	if accounts.lastSubscriptionUpdateAccountID != "acc-1" {
		t.Fatalf("expected account subscription update")
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected one runtime provisioning call, got %d", provisioner.calls)
	}
}

func TestMarkMailboxPaidActivatesKeyBoundMailboxWithoutAccount(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-key-1": {
				ID:               "mbx-key-1",
				IMAPUsername:     "mbx_key",
				IMAPPassword:     "pass",
				PaymentSessionID: "sess-key-1",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, err := service.MarkMailboxPaid(context.Background(), "sess-key-1")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", mailbox.Status)
	}
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected one runtime provisioning call, got %d", provisioner.calls)
	}
}

func TestResolveIMAPRejectsExpiredMailbox(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:          "mbx-1",
				AccountID:   "acc-1",
				Status:      domain.MailboxStatusActive,
				PaidAt:      ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:   &expiredAt,
				AccessToken: "token-1",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acc-1": {ID: "acc-1", SubscriptionExpiresAt: ptrTime(time.Now().UTC().Add(-time.Minute))},
		},
	}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	_, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if !errors.Is(err, ports.ErrMailboxNotUsable) {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
}

func TestResolveIMAPAllowsPendingMailboxWhenAccountSubscribed(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusPendingPayment,
				AccessToken:  "token-1",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{"acc-1": {ID: "acc-1", SubscriptionExpiresAt: &future}}}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	result, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("ResolveIMAPByToken failed: %v", err)
	}
	if result.Username != "u" {
		t.Fatalf("expected IMAP username u, got %s", result.Username)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner called once")
	}
}

func TestResolveIMAPByKeyReturnsActiveMailbox(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mx.example.com", "imap.example.com", 143)

	result, err := service.ResolveIMAPByKey(context.Background(), ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	})
	if err != nil {
		t.Fatalf("ResolveIMAPByKey failed: %v", err)
	}
	if result.Email != "mbx_abc@mx.example.com" {
		t.Fatalf("expected email to use mail domain, got %q", result.Email)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioner called once")
	}
}

func TestResolveIMAPByKeyRejectsUnusableMailbox(t *testing.T) {
	expired := time.Now().UTC().Add(-time.Minute)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-2": {
				ID:             "mbx-2",
				KeyFingerprint: "edproof:key-2",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-time.Hour)),
				ExpiresAt:      &expired,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mx.example.com", "imap.example.com", 143)

	_, err := service.ResolveIMAPByKey(context.Background(), ports.VerifiedKey{
		Fingerprint: "edproof:key-2",
		Algorithm:   "ed25519",
	})
	if !errors.Is(err, ports.ErrMailboxNotUsable) {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
}

func TestResolveIMAPReturnsMailboxAddressUsingMailDomain(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusActive,
				AccessToken:  "token-1",
				IMAPHost:     "imap.example.com",
				IMAPPort:     143,
				IMAPUsername: "mbx_abc",
				IMAPPassword: "p",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{"acc-1": {ID: "acc-1", SubscriptionExpiresAt: &future}}}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mx.example.com", "imap.example.com", 143)

	result, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if err != nil {
		t.Fatalf("ResolveIMAPByToken failed: %v", err)
	}
	if result.Email != "mbx_abc@mx.example.com" {
		t.Fatalf("expected mailbox email to use mail domain, got %q", result.Email)
	}
}

func TestListMessagesByTokenReturnsReaderMessages(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusActive,
				AccessToken:  "token-1",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{"acc-1": {ID: "acc-1", SubscriptionExpiresAt: &future}}}
	reader := &fakeMailReader{messages: []ports.IMAPMessage{{UID: 1, Subject: "hello", From: "a@b"}}}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, reader, "mail.test.local", "imap.test.local", 1143)

	messages, err := service.ListMessagesByToken(context.Background(), "token-1", 20, true, true)
	if err != nil {
		t.Fatalf("ListMessagesByToken failed: %v", err)
	}
	if len(messages) != 1 || messages[0].Subject != "hello" {
		t.Fatalf("unexpected messages result: %+v", messages)
	}
}

func TestResolveAccessByTokenWorksForKeyBoundMailbox(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-kb": {
				ID:           "mbx-kb",
				AccountID:    "",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:    &future,
				AccessToken:  "token-kb",
				IMAPHost:     "imap.example.com",
				IMAPPort:     143,
				IMAPUsername: "mbx_abc",
				IMAPPassword: "secret",
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mx.example.com", "imap.example.com", 143)

	result, err := service.ResolveIMAPByToken(context.Background(), "token-kb")
	if err != nil {
		t.Fatalf("ResolveIMAPByToken failed for key-bound mailbox: %v", err)
	}
	if result.Username != "mbx_abc" {
		t.Fatalf("expected IMAP username mbx_abc, got %s", result.Username)
	}
	if result.AccessToken != "token-kb" {
		t.Fatalf("expected AccessToken token-kb, got %s", result.AccessToken)
	}
}

func TestResolveAccessByTokenRejectsExpiredKeyBoundMailbox(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-kb": {
				ID:          "mbx-kb",
				AccountID:   "",
				Status:      domain.MailboxStatusActive,
				PaidAt:      ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:   &expiredAt,
				AccessToken: "token-kb",
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	_, err := service.ResolveIMAPByToken(context.Background(), "token-kb")
	if !errors.Is(err, ports.ErrMailboxNotUsable) {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
}

func TestListMessagesByTokenWorksForKeyBoundMailbox(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-kb": {
				ID:           "mbx-kb",
				AccountID:    "",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:    &future,
				AccessToken:  "token-kb",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	reader := &fakeMailReader{messages: []ports.IMAPMessage{{UID: 1, Subject: "hello"}}}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, reader, "mail.test.local", "imap.test.local", 1143)

	messages, err := service.ListMessagesByToken(context.Background(), "token-kb", 20, true, false)
	if err != nil {
		t.Fatalf("ListMessagesByToken failed for key-bound mailbox: %v", err)
	}
	if len(messages) != 1 || messages[0].Subject != "hello" {
		t.Fatalf("unexpected messages result: %+v", messages)
	}
}

func TestGetMessageByUIDTokenWorksForKeyBoundMailbox(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-kb": {
				ID:           "mbx-kb",
				AccountID:    "",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:    &future,
				AccessToken:  "token-kb",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	reader := &fakeMailReader{messageByUID: map[uint32]ports.IMAPMessage{3: {UID: 3, Subject: "keyed"}}}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, reader, "mail.test.local", "imap.test.local", 1143)

	message, err := service.GetMessageByUIDToken(context.Background(), "token-kb", 3, true)
	if err != nil {
		t.Fatalf("GetMessageByUIDToken failed for key-bound mailbox: %v", err)
	}
	if message == nil || message.UID != 3 {
		t.Fatalf("unexpected message result: %+v", message)
	}
}

func TestResolveAccessResultIncludesAccessToken(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				AccessToken:    "my-access-token",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-time.Minute)),
				ExpiresAt:      &future,
				IMAPHost:       "imap.example.com",
				IMAPPort:       143,
				IMAPUsername:   "mbx_abc",
				IMAPPassword:   "secret",
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mx.example.com", "imap.example.com", 143)

	result, err := service.ResolveIMAPByKey(context.Background(), ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	})
	if err != nil {
		t.Fatalf("ResolveIMAPByKey failed: %v", err)
	}
	if result.AccessToken != "my-access-token" {
		t.Fatalf("expected AccessToken my-access-token, got %q", result.AccessToken)
	}
}

func TestGetMessageByUIDTokenReturnsSingleMessage(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusActive,
				AccessToken:  "token-1",
				IMAPHost:     "imap",
				IMAPPort:     143,
				IMAPUsername: "u",
				IMAPPassword: "p",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{"acc-1": {ID: "acc-1", SubscriptionExpiresAt: &future}}}
	reader := &fakeMailReader{messageByUID: map[uint32]ports.IMAPMessage{7: {UID: 7, Subject: "single"}}}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, reader, "mail.test.local", "imap.test.local", 1143)

	message, err := service.GetMessageByUIDToken(context.Background(), "token-1", 7, true)
	if err != nil {
		t.Fatalf("GetMessageByUIDToken failed: %v", err)
	}
	if message == nil || message.UID != 7 {
		t.Fatalf("unexpected message result: %+v", message)
	}
}

func TestCreateMailboxMultipleForSponsoredAccount(t *testing.T) {
	now := time.Now().UTC().Add(24 * time.Hour)
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	provisioner := &fakeMailRuntimeProvisioner{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	account := &domain.Account{ID: "acc-1", OwnerEmail: "sponsor@example.com", SubscriptionExpiresAt: &now}

	first, created1, err := svc.CreateMailbox(context.Background(), CreateMailboxRequest{Account: account})
	if err != nil {
		t.Fatalf("first CreateMailbox failed: %v", err)
	}
	if !created1 {
		t.Fatalf("expected first mailbox to be newly created")
	}

	second, created2, err := svc.CreateMailbox(context.Background(), CreateMailboxRequest{Account: account})
	if err != nil {
		t.Fatalf("second CreateMailbox failed: %v", err)
	}
	if !created2 {
		t.Fatalf("expected second mailbox to be newly created")
	}

	if first.ID == second.ID {
		t.Fatalf("expected different mailbox IDs, both are %q", first.ID)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if provisioner.calls != 2 {
		t.Fatalf("expected two provisions, got %d", provisioner.calls)
	}
}

// --- Gift coupon tests ---

func TestClaimMailboxWithValidCouponSetsDiscountAndGrantedMonths(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier,
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143,
		GiftCouponConfig{DiscountID: "disc-123", CouponCode: "OPENCLAWS"})

	mailbox, created, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:gift-key",
		Algorithm:   "ed25519",
	}, "openclaws")
	if err != nil {
		t.Fatalf("ClaimMailbox with coupon failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new mailbox to be created")
	}
	if mailbox.GrantedMonths != 3 {
		t.Fatalf("expected GrantedMonths=3, got %d", mailbox.GrantedMonths)
	}
	if payment.lastReq.DiscountID != "disc-123" {
		t.Fatalf("expected DiscountID=disc-123, got %q", payment.lastReq.DiscountID)
	}
	if !mailbox.CouponUsed {
		t.Fatalf("expected CouponUsed=true")
	}
}

func TestClaimMailboxWithInvalidCouponReturnsError(t *testing.T) {
	repo := &fakeMailboxRepo{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143,
		GiftCouponConfig{DiscountID: "disc-123", CouponCode: "OPENCLAWS"})

	_, _, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:bad-key",
		Algorithm:   "ed25519",
	}, "WRONGCODE")
	if !errors.Is(err, ports.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}

func TestClaimMailboxWithCouponButNoConfigReturnsError(t *testing.T) {
	repo := &fakeMailboxRepo{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	_, _, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:noconfig-key",
		Algorithm:   "ed25519",
	}, "OPENCLAWS")
	if !errors.Is(err, ports.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got %v", err)
	}
}

func TestClaimMailboxCouponAlreadyUsedBySameKey(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:used-key": {
				ID:             "mbx-used",
				KeyFingerprint: "edproof:used-key",
				Status:         domain.MailboxStatusExpired,
				CouponUsed:     true,
			},
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143,
		GiftCouponConfig{DiscountID: "disc-123", CouponCode: "OPENCLAWS"})

	_, _, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:used-key",
		Algorithm:   "ed25519",
	}, "OPENCLAWS")
	if !errors.Is(err, ports.ErrCouponAlreadyUsed) {
		t.Fatalf("expected ErrCouponAlreadyUsed, got %v", err)
	}
}

func TestClaimMailboxWithoutCouponNormalFlow(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143,
		GiftCouponConfig{DiscountID: "disc-123", CouponCode: "OPENCLAWS"})

	mailbox, _, err := svc.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:normal-key",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox without coupon failed: %v", err)
	}
	if mailbox.GrantedMonths != 0 {
		t.Fatalf("expected GrantedMonths=0, got %d", mailbox.GrantedMonths)
	}
	if payment.lastReq.DiscountID != "" {
		t.Fatalf("expected empty DiscountID, got %q", payment.lastReq.DiscountID)
	}
}

func TestMarkMailboxPaidWithGrantedMonths3(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-gift": {
				ID:               "mbx-gift",
				PaymentSessionID: "sess-gift",
				Status:           domain.MailboxStatusPendingPayment,
				GrantedMonths:    3,
			},
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	mailbox, err := svc.MarkMailboxPaid(context.Background(), "sess-gift")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", mailbox.Status)
	}
	expected := now.AddDate(0, 3, 0)
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected ExpiresAt to be set")
	}
	diff := mailbox.ExpiresAt.Sub(expected)
	if diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expected ExpiresAt ~%v, got %v (diff %v)", expected, *mailbox.ExpiresAt, diff)
	}
}

func TestMarkMailboxPaidWithGrantedMonths0DefaultsTo1(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-normal": {
				ID:               "mbx-normal",
				PaymentSessionID: "sess-normal",
				Status:           domain.MailboxStatusPendingPayment,
				GrantedMonths:    0,
			},
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	mailbox, err := svc.MarkMailboxPaid(context.Background(), "sess-normal")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	expected := now.AddDate(0, 1, 0)
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected ExpiresAt to be set")
	}
	diff := mailbox.ExpiresAt.Sub(expected)
	if diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expected ExpiresAt ~%v, got %v (diff %v)", expected, *mailbox.ExpiresAt, diff)
	}
}

func TestExpireMailboxesSweepsExpiredOnly(t *testing.T) {
	past := time.Now().UTC().Add(-24 * time.Hour)
	future := time.Now().UTC().Add(24 * time.Hour)

	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:expired1": {
				ID:             "mbx-expired1",
				KeyFingerprint: "edproof:expired1",
				Status:         domain.MailboxStatusActive,
				ExpiresAt:      &past,
			},
			"edproof:active": {
				ID:             "mbx-active",
				KeyFingerprint: "edproof:active",
				Status:         domain.MailboxStatusActive,
				ExpiresAt:      &future,
			},
			"edproof:pending": {
				ID:             "mbx-pending",
				KeyFingerprint: "edproof:pending",
				Status:         domain.MailboxStatusPendingPayment,
				ExpiresAt:      &past,
			},
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	n, err := svc.ExpireMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ExpireMailboxes failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired, got %d", n)
	}

	// The expired mailbox should be flipped.
	if repo.byKeyFingerprint["edproof:expired1"].Status != domain.MailboxStatusExpired {
		t.Fatalf("expected mbx-expired1 to be expired, got %s", repo.byKeyFingerprint["edproof:expired1"].Status)
	}
	// The active mailbox should be untouched.
	if repo.byKeyFingerprint["edproof:active"].Status != domain.MailboxStatusActive {
		t.Fatalf("expected mbx-active to remain active, got %s", repo.byKeyFingerprint["edproof:active"].Status)
	}
	// The pending mailbox should be untouched.
	if repo.byKeyFingerprint["edproof:pending"].Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected mbx-pending to remain pending, got %s", repo.byKeyFingerprint["edproof:pending"].Status)
	}
}

func TestExpireMailboxesReturnsZeroWhenNothingExpired(t *testing.T) {
	future := time.Now().UTC().Add(24 * time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:ok": {
				ID:             "mbx-ok",
				KeyFingerprint: "edproof:ok",
				Status:         domain.MailboxStatusActive,
				ExpiresAt:      &future,
			},
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	n, err := svc.ExpireMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ExpireMailboxes failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 expired, got %d", n)
	}
}

type fakeMailboxRepo struct {
	pendingByAccount              map[string]*domain.Mailbox
	created                       []*domain.Mailbox
	byStripeSession               map[string]*domain.Mailbox
	byActivationTokenHash         map[string]*domain.Mailbox
	byAccessToken                 map[string]*domain.Mailbox
	byKeyFingerprint              map[string]*domain.Mailbox
	activeOrPendingByBillingEmail map[string]*domain.Mailbox
	updated                       *domain.Mailbox
}

type fakeMailboxAccountRepo struct {
	byID                            map[string]*domain.Account
	lastSubscriptionUpdateAccountID string
	lastSubscriptionUpdateExpiresAt time.Time
}

func (f *fakeMailboxAccountRepo) Create(_ context.Context, _ *domain.Account) error { return nil }

func (f *fakeMailboxAccountRepo) GetByID(_ context.Context, accountID string) (*domain.Account, error) {
	if f.byID != nil {
		if item, ok := f.byID[accountID]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) GetByOwnerEmail(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) GetByAPIToken(_ context.Context, _ string) (*domain.Account, error) {
	return nil, ports.ErrAccountNotFound
}

func (f *fakeMailboxAccountRepo) UpdateAPIToken(_ context.Context, _ string, _ string) error {
	return nil
}

func (f *fakeMailboxAccountRepo) UpdateSubscriptionExpiresAt(_ context.Context, accountID string, expiresAt time.Time) error {
	f.lastSubscriptionUpdateAccountID = accountID
	f.lastSubscriptionUpdateExpiresAt = expiresAt
	if f.byID == nil {
		f.byID = map[string]*domain.Account{}
	}
	if item, ok := f.byID[accountID]; ok {
		item.SubscriptionExpiresAt = &expiresAt
	}
	return nil
}

func (f *fakeMailboxAccountRepo) ClearSubscriptionExpiresAt(_ context.Context) (int, error) {
	count := 0
	for _, acc := range f.byID {
		if acc != nil && acc.SubscriptionExpiresAt != nil {
			acc.SubscriptionExpiresAt = nil
			count++
		}
	}
	return count, nil
}

func (f *fakeMailboxRepo) Create(_ context.Context, mailbox *domain.Mailbox) error {
	f.created = append(f.created, mailbox)
	if f.byKeyFingerprint == nil {
		f.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	if mailbox.KeyFingerprint != "" {
		f.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
	}
	if f.byActivationTokenHash == nil {
		f.byActivationTokenHash = map[string]*domain.Mailbox{}
	}
	if mailbox.ActivationTokenHash != "" {
		f.byActivationTokenHash[mailbox.ActivationTokenHash] = mailbox
	}
	return nil
}

func (f *fakeMailboxRepo) Update(_ context.Context, mailbox *domain.Mailbox) error {
	f.updated = mailbox
	if f.byKeyFingerprint == nil {
		f.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	if mailbox.KeyFingerprint != "" {
		f.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
	}
	if f.byStripeSession == nil {
		f.byStripeSession = map[string]*domain.Mailbox{}
	}
	if mailbox.PaymentSessionID != "" {
		f.byStripeSession[mailbox.PaymentSessionID] = mailbox
	}
	if f.byActivationTokenHash == nil {
		f.byActivationTokenHash = map[string]*domain.Mailbox{}
	}
	if mailbox.ActivationTokenHash != "" {
		f.byActivationTokenHash[mailbox.ActivationTokenHash] = mailbox
	}
	return nil
}

func (f *fakeMailboxRepo) GetByID(_ context.Context, _ string) (*domain.Mailbox, error) {
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) ListByAccountID(_ context.Context, _ string) ([]domain.Mailbox, error) {
	return nil, nil
}

func (f *fakeMailboxRepo) ListPendingPayment(_ context.Context) ([]domain.Mailbox, error) {
	var result []domain.Mailbox
	for _, mb := range f.byStripeSession {
		if mb != nil && mb.Status == domain.MailboxStatusPendingPayment {
			result = append(result, *mb)
		}
	}
	return result, nil
}

func (f *fakeMailboxRepo) GetPendingByAccountID(_ context.Context, accountID string) (*domain.Mailbox, error) {
	if item, ok := f.pendingByAccount[accountID]; ok {
		return item, nil
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByPaymentSessionID(_ context.Context, sessionID string) (*domain.Mailbox, error) {
	if f.byStripeSession != nil {
		if item, ok := f.byStripeSession[sessionID]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByActivationTokenHash(_ context.Context, tokenHash string) (*domain.Mailbox, error) {
	if f.byActivationTokenHash != nil {
		if item, ok := f.byActivationTokenHash[tokenHash]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByAccessToken(_ context.Context, accessToken string) (*domain.Mailbox, error) {
	if f.byAccessToken != nil {
		if item, ok := f.byAccessToken[accessToken]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) GetByKeyFingerprint(_ context.Context, keyFingerprint string) (*domain.Mailbox, error) {
	if f.byKeyFingerprint != nil {
		if item, ok := f.byKeyFingerprint[keyFingerprint]; ok {
			return item, nil
		}
	}
	return nil, ports.ErrMailboxNotFound
}

func (f *fakeMailboxRepo) ListActiveExpired(_ context.Context, now time.Time) ([]domain.Mailbox, error) {
	var result []domain.Mailbox
	for _, mb := range f.byKeyFingerprint {
		if mb != nil && mb.Status == domain.MailboxStatusActive && mb.ExpiresAt != nil && !mb.ExpiresAt.After(now) {
			result = append(result, *mb)
		}
	}
	for _, mb := range f.created {
		if mb != nil && mb.Status == domain.MailboxStatusActive && mb.ExpiresAt != nil && !mb.ExpiresAt.After(now) {
			result = append(result, *mb)
		}
	}
	return result, nil
}

// allMailboxes returns every mailbox the fake holds, deduplicated by ID.
func (f *fakeMailboxRepo) allMailboxes() []*domain.Mailbox {
	seen := map[string]*domain.Mailbox{}
	add := func(mb *domain.Mailbox) {
		if mb == nil {
			return
		}
		if _, ok := seen[mb.ID]; ok {
			return
		}
		seen[mb.ID] = mb
	}
	for _, mb := range f.byKeyFingerprint {
		add(mb)
	}
	for _, mb := range f.created {
		add(mb)
	}
	for _, mb := range f.byStripeSession {
		add(mb)
	}
	for _, mb := range f.byActivationTokenHash {
		add(mb)
	}
	for _, mb := range f.byAccessToken {
		add(mb)
	}
	for _, mb := range f.pendingByAccount {
		add(mb)
	}

	result := make([]*domain.Mailbox, 0, len(seen))
	for _, mb := range seen {
		result = append(result, mb)
	}
	return result
}

func (f *fakeMailboxRepo) ListActive(_ context.Context) ([]domain.Mailbox, error) {
	var result []domain.Mailbox
	for _, mb := range f.allMailboxes() {
		if mb.Status == domain.MailboxStatusActive {
			result = append(result, *mb)
		}
	}
	return result, nil
}

func (f *fakeMailboxRepo) ClearActiveExpiries(_ context.Context) (int, error) {
	count := 0
	for _, mb := range f.allMailboxes() {
		if mb.Status == domain.MailboxStatusActive {
			mb.ExpiresAt = nil
			count++
		}
	}
	return count, nil
}

type fakePaymentGateway struct {
	calls             int
	getCalls          int
	lastReq           ports.PaymentLinkRequest
	getPaymentSession func(context.Context, string) (*ports.PaymentSession, error)
}

func (f *fakePaymentGateway) CreatePaymentLink(_ context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	f.calls++
	f.lastReq = req
	return &ports.PaymentLink{SessionID: "sess-1", URL: "http://pay/1"}, nil
}

func (f *fakePaymentGateway) GetPaymentSession(ctx context.Context, sessionID string) (*ports.PaymentSession, error) {
	f.getCalls++
	if f.getPaymentSession != nil {
		return f.getPaymentSession(ctx, sessionID)
	}
	return &ports.PaymentSession{
		SessionID: sessionID,
		Status:    ports.PaymentSessionStatusSucceeded,
	}, nil
}

type fakeMailboxTokenGenerator struct {
	token string
}

func (f fakeMailboxTokenGenerator) NewToken(_ int) (string, error) {
	return f.token, nil
}

type fakeMailboxNotifier struct {
	calls int
}

type fakeMailRuntimeProvisioner struct {
	calls int
}

type fakeMailReader struct {
	messages        []ports.IMAPMessage
	messageByUID    map[uint32]ports.IMAPMessage
	lastIncludeBody bool
}

func (f *fakeMailReader) ListMessages(_ context.Context, _ string, _ int, _ string, _ string, _ int, _ bool, includeBody bool) ([]ports.IMAPMessage, error) {
	f.lastIncludeBody = includeBody
	if f.messages == nil {
		return []ports.IMAPMessage{}, nil
	}
	return f.messages, nil
}

func (f *fakeMailReader) GetMessageByUID(_ context.Context, _ string, _ int, _ string, _ string, uid uint32, includeBody bool) (*ports.IMAPMessage, error) {
	f.lastIncludeBody = includeBody
	if f.messageByUID == nil {
		return nil, nil
	}
	item, ok := f.messageByUID[uid]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (f *fakeMailRuntimeProvisioner) EnsureMailbox(_ context.Context, _ *domain.Mailbox) error {
	f.calls++
	return nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func (f *fakeMailboxNotifier) SendPaymentLink(_ context.Context, _ string, _ string, _ string) error {
	f.calls++
	return nil
}

func (f *fakeMailboxNotifier) SendActivationLink(_ context.Context, _ string, _ string, _ string) error {
	f.calls++
	return nil
}

func (f *fakeMailboxNotifier) SendRecoveryLink(_ context.Context, _ string, _ string) error {
	return nil
}

func (f *fakeMailboxNotifier) SendSupportMessage(_ context.Context, _ ports.SupportMessageParams) error {
	f.calls++
	return nil
}

func TestReconcilePendingPayments(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-confirmed": {
				ID:               "mbx-1",
				PaymentSessionID: "sess-confirmed",
				Status:           domain.MailboxStatusPendingPayment,
				GrantedMonths:    1,
			},
			"sess-open": {
				ID:               "mbx-2",
				PaymentSessionID: "sess-open",
				Status:           domain.MailboxStatusPendingPayment,
				GrantedMonths:    1,
			},
		},
	}
	gateway := &reconcileFakeGateway{
		sessions: map[string]ports.PaymentSessionStatus{
			"sess-confirmed": ports.PaymentSessionStatusConfirmed,
			"sess-open":      ports.PaymentSessionStatusOpen,
		},
	}
	svc := NewMailboxService(repo, &fakeMailboxAccountRepo{}, gateway, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	results, err := svc.ReconcilePendingPayments(context.Background())
	if err != nil {
		t.Fatalf("ReconcilePendingPayments failed: %v", err)
	}

	activatedCount := 0
	noActionCount := 0
	for _, r := range results {
		switch r.Action {
		case "activated":
			activatedCount++
			if r.MailboxID != "mbx-1" {
				t.Errorf("expected mbx-1 to be activated, got %s", r.MailboxID)
			}
		case "no_action":
			noActionCount++
		}
	}

	if activatedCount != 1 {
		t.Errorf("expected 1 activated, got %d", activatedCount)
	}
	if noActionCount != 1 {
		t.Errorf("expected 1 no_action, got %d", noActionCount)
	}
}

type reconcileFakeGateway struct {
	sessions map[string]ports.PaymentSessionStatus
}

func (g *reconcileFakeGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	return &ports.PaymentLink{SessionID: "sess-new", URL: "http://pay/new"}, nil
}

func (g *reconcileFakeGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	status, ok := g.sessions[sessionID]
	if !ok {
		return nil, ports.ErrMailboxNotFound
	}
	return &ports.PaymentSession{SessionID: sessionID, Status: status}, nil
}

func TestClaimMailboxFreeModeSkipsPaymentAndEmailsActivationLink(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "act-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new mailbox to be created")
	}
	if mailbox.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected pending_payment status, got %s", mailbox.Status)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation in free mode, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email, got %d", notifier.calls)
	}
	if mailbox.PaymentURL != "" || mailbox.PaymentSessionID != "" {
		t.Fatalf("expected no payment fields in free mode, got url=%q session=%q", mailbox.PaymentURL, mailbox.PaymentSessionID)
	}
	if mailbox.ActivationTokenHash != hashToken("act-token") {
		t.Fatalf("expected activation token hash, got %q", mailbox.ActivationTokenHash)
	}
	if mailbox.ActivationExpiresAt == nil || !mailbox.ActivationExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("expected activation expiry in the future, got %v", mailbox.ActivationExpiresAt)
	}
	wantURL := "http://test.local/v1/mailboxes/activate?token=act-token"
	if mailbox.ActivationURL != wantURL {
		t.Fatalf("expected activation url %q, got %q", wantURL, mailbox.ActivationURL)
	}
}

func TestClaimMailboxFreeModeIgnoresCoupon(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "act-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143, GiftCouponConfig{DiscountID: "disc-1", CouponCode: "GIFT10"})
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	}, "GIFT10")
	if err != nil {
		t.Fatalf("expected valid configured coupon to be ignored in free mode, got %v", err)
	}
	if !created {
		t.Fatalf("expected new mailbox")
	}
	if mailbox.CouponUsed {
		t.Fatalf("expected no coupon applied in free mode")
	}
	if mailbox.GrantedMonths != 0 {
		t.Fatalf("expected no granted months in free mode, got %d", mailbox.GrantedMonths)
	}

	// An unknown coupon must not produce a coupon error in free mode.
	if _, _, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-2",
		Algorithm:   "ed25519",
	}, "BOGUS"); err != nil {
		t.Fatalf("expected unknown coupon to be ignored in free mode, got %v", err)
	}
}

func TestClaimMailboxFreeModeReclaimRegeneratesActivationToken(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:                  "mbx-1",
				KeyFingerprint:      "edproof:key-1",
				OwnerEmail:          "old@example.com",
				BillingEmail:        "old@example.com",
				Status:              domain.MailboxStatusPendingPayment,
				ActivationTokenHash: hashToken("old-token"),
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "new-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.ActivationTokenHash != hashToken("new-token") {
		t.Fatalf("expected regenerated activation token hash, got %q", mailbox.ActivationTokenHash)
	}
	if !strings.Contains(mailbox.ActivationURL, "token=new-token") {
		t.Fatalf("expected activation url to carry the new token, got %q", mailbox.ActivationURL)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email, got %d", notifier.calls)
	}

	if _, err := service.ActivateMailboxByActivationToken(context.Background(), "old-token"); !errors.Is(err, ports.ErrActivationTokenInvalid) {
		t.Fatalf("expected old token to be invalid after re-claim, got %v", err)
	}
}

func TestClaimMailboxFreeModeReclaimExpiredMailboxRegeneratesToken(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusExpired,
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "new-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-1",
		Algorithm:   "ed25519",
	}, "")
	if err != nil {
		t.Fatalf("ClaimMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing mailbox reuse, got created=true")
	}
	if mailbox.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected re-claimed mailbox to be pending again, got %s", mailbox.Status)
	}
	if mailbox.ActivationTokenHash != hashToken("new-token") {
		t.Fatalf("expected regenerated activation token hash, got %q", mailbox.ActivationTokenHash)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email, got %d", notifier.calls)
	}
}

func TestActivateMailboxByActivationTokenActivatesPendingMailbox(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byActivationTokenHash: map[string]*domain.Mailbox{
			hashToken("raw-token"): {
				ID:                  "mbx-1",
				KeyFingerprint:      "edproof:key-1",
				Status:              domain.MailboxStatusPendingPayment,
				ActivationTokenHash: hashToken("raw-token"),
				ActivationExpiresAt: &future,
			},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	result, err := service.ActivateMailboxByActivationToken(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("ActivateMailboxByActivationToken failed: %v", err)
	}
	if result.AlreadyActive {
		t.Fatalf("expected a fresh activation, got already-active")
	}
	if result.Mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", result.Mailbox.Status)
	}
	if result.Mailbox.PaidAt == nil {
		t.Fatalf("expected PaidAt to be set")
	}
	if result.Mailbox.ExpiresAt != nil {
		t.Fatalf("expected nil ExpiresAt in free mode, got %v", result.Mailbox.ExpiresAt)
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected mailbox provisioning, got %d calls", provisioner.calls)
	}

	// Resolve now works (Covers AE2).
	res, err := service.ResolveAccessByKey(context.Background(), ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"}, "imap")
	if err != nil {
		t.Fatalf("resolve after activation failed: %v", err)
	}
	if res.MailboxID != "mbx-1" {
		t.Fatalf("expected resolve to return mbx-1, got %q", res.MailboxID)
	}
}

func TestActivateMailboxByActivationTokenRejectsExpiredToken(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byActivationTokenHash: map[string]*domain.Mailbox{
			hashToken("raw-token"): {
				ID:                  "mbx-1",
				Status:              domain.MailboxStatusPendingPayment,
				ActivationTokenHash: hashToken("raw-token"),
				ActivationExpiresAt: &past,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	if _, err := service.ActivateMailboxByActivationToken(context.Background(), "raw-token"); !errors.Is(err, ports.ErrActivationTokenInvalid) {
		t.Fatalf("expected expired token to be rejected, got %v", err)
	}
	mb, _ := repo.GetByActivationTokenHash(context.Background(), hashToken("raw-token"))
	if mb.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected expired-token attempt to leave status unchanged, got %s", mb.Status)
	}
}

func TestActivateMailboxByActivationTokenRejectsUnknownToken(t *testing.T) {
	service := NewMailboxService(&fakeMailboxRepo{}, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	if _, err := service.ActivateMailboxByActivationToken(context.Background(), "nope"); !errors.Is(err, ports.ErrActivationTokenInvalid) {
		t.Fatalf("expected unknown token to be rejected, got %v", err)
	}
}

func TestActivateMailboxByActivationTokenIdempotentWhenAlreadyActive(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byActivationTokenHash: map[string]*domain.Mailbox{
			hashToken("raw-token"): {
				ID:                  "mbx-1",
				Status:              domain.MailboxStatusActive,
				PaidAt:              &now,
				ActivationTokenHash: hashToken("raw-token"),
			},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	result, err := service.ActivateMailboxByActivationToken(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("ActivateMailboxByActivationToken failed: %v", err)
	}
	if !result.AlreadyActive {
		t.Fatalf("expected already-active result on second click")
	}
	if provisioner.calls != 1 {
		t.Fatalf("expected provisioning on idempotent click, got %d calls", provisioner.calls)
	}
}

func TestCreateMailboxFreeModeCreatesActivationPendingMailbox(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "act-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com"},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if !created {
		t.Fatalf("expected new mailbox to be created")
	}
	if mailbox.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected pending_payment status, got %s", mailbox.Status)
	}
	if mailbox.ExpiresAt != nil {
		t.Fatalf("expected no expiry in free mode, got %v", mailbox.ExpiresAt)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation in free mode, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email, got %d", notifier.calls)
	}
	if mailbox.ActivationURL == "" {
		t.Fatalf("expected an activation url on the mailbox")
	}
}

func TestCreateMailboxFreeModeRegeneratesLinkForExistingPending(t *testing.T) {
	repo := &fakeMailboxRepo{
		pendingByAccount: map[string]*domain.Mailbox{
			"acc-1": {
				ID:         "mbx-1",
				AccountID:  "acc-1",
				OwnerEmail: "owner@example.com",
				Status:     domain.MailboxStatusPendingPayment,
			},
		},
	}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "act-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	mailbox, created, err := service.CreateMailbox(context.Background(), CreateMailboxRequest{
		Account: &domain.Account{ID: "acc-1", OwnerEmail: "owner@example.com"},
	})
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}
	if created {
		t.Fatalf("expected existing pending mailbox reuse, got created=true")
	}
	if mailbox.ActivationTokenHash != hashToken("act-token") {
		t.Fatalf("expected regenerated activation token hash, got %q", mailbox.ActivationTokenHash)
	}
	if payment.calls != 0 {
		t.Fatalf("expected no payment link creation, got %d", payment.calls)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email, got %d", notifier.calls)
	}
}

func TestResolveAccessByTokenFreeModeBypassesAccountSubscription(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"tok-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				AccessToken:  "tok-1",
				Status:       domain.MailboxStatusActive,
				PaidAt:       &now,
				ExpiresAt:    nil,
				IMAPHost:     "imap.test.local",
				IMAPPort:     1143,
				IMAPUsername: "mbx-1",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{
		"acc-1": {ID: "acc-1", OwnerEmail: "owner@example.com"},
	}}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	res, err := service.ResolveAccessByToken(context.Background(), "tok-1", "imap")
	if err != nil {
		t.Fatalf("expected resolve success in free mode despite no subscription, got %v", err)
	}
	if res.MailboxID != "mbx-1" {
		t.Fatalf("expected mailbox mbx-1, got %q", res.MailboxID)
	}
}

func TestResolveAccessByTokenGrandfathersFreeMailboxAfterReEnable(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"tok-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				AccessToken:  "tok-1",
				Status:       domain.MailboxStatusActive,
				PaidAt:       &now,
				ExpiresAt:    nil,
				IMAPHost:     "imap.test.local",
				IMAPPort:     1143,
				IMAPUsername: "mbx-1",
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{byID: map[string]*domain.Account{
		"acc-1": {ID: "acc-1", OwnerEmail: "owner@example.com"},
	}}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "x"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	// freeMode intentionally left false (paid re-enabled): the mailbox's nil ExpiresAt must still bypass the account gate.

	res, err := service.ResolveAccessByToken(context.Background(), "tok-1", "imap")
	if err != nil {
		t.Fatalf("expected grandfathered free mailbox to stay usable after re-enable, got %v", err)
	}
	if res.MailboxID != "mbx-1" {
		t.Fatalf("expected mailbox mbx-1, got %q", res.MailboxID)
	}
}

func TestMarkMailboxPaidFreeModeIsNoOp(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-free": {
				ID:               "mbx-free",
				IMAPUsername:     "mbx_free",
				IMAPPassword:     "pass",
				PaymentSessionID: "sess-free",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	provisioner := &fakeMailRuntimeProvisioner{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, provisioner, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	mailbox, err := service.MarkMailboxPaid(context.Background(), "sess-free")
	if err != nil {
		t.Fatalf("MarkMailboxPaid in free mode should no-op without error, got %v", err)
	}
	if mailbox != nil {
		t.Fatalf("expected nil mailbox from free-mode no-op, got %+v", mailbox)
	}
	if got := repo.byStripeSession["sess-free"]; got.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected mailbox to stay pending, got %s", got.Status)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
	if provisioner.calls != 0 {
		t.Fatalf("expected no provisioning call in free mode, got %d", provisioner.calls)
	}
}

func TestRenewMailboxFreeModeIsNoOp(t *testing.T) {
	repo := &fakeMailboxRepo{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	err := service.RenewMailbox(context.Background(), "mbx-any", time.Now().UTC(), time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("RenewMailbox in free mode should no-op without error, got %v", err)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

// Covers AE5: a subscription.revoked event path (ExpireMailboxByID) in free mode
// leaves an active mailbox active.
func TestExpireMailboxByIDFreeModeLeavesActiveMailboxActive(t *testing.T) {
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-time.Hour)),
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	if err := service.ExpireMailboxByID(context.Background(), "mbx-1"); err != nil {
		t.Fatalf("ExpireMailboxByID in free mode should no-op, got %v", err)
	}
	if got := repo.byKeyFingerprint["edproof:key-1"]; got.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to stay active, got %s", got.Status)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

func TestReconcilePendingPaymentsFreeModeTouchesNoRows(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-p1": {
				ID:               "mbx-p1",
				PaymentSessionID: "sess-p1",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	payment := &fakePaymentGateway{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	results, err := service.ReconcilePendingPayments(context.Background())
	if err != nil {
		t.Fatalf("ReconcilePendingPayments in free mode should no-op, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results in free mode, got %d entries", len(results))
	}
	if payment.getCalls != 0 {
		t.Fatalf("expected no gateway session lookups in free mode, got %d", payment.getCalls)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

func TestExpireMailboxesFreeModeFlipsNoRows(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:      &past,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	n, err := service.ExpireMailboxes(context.Background())
	if err != nil {
		t.Fatalf("ExpireMailboxes in free mode should no-op, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 expired in free mode, got %d", n)
	}
	if got := repo.byKeyFingerprint["edproof:key-1"]; got.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to stay active, got %s", got.Status)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

func TestResolveAccessByKeyFreeModeDoesNotExpireStaleMailbox(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:      &past,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	_, err := service.ResolveAccessByKey(context.Background(), ports.VerifiedKey{Fingerprint: "edproof:key-1", Algorithm: "ed25519"}, "imap")
	if !errors.Is(err, ports.ErrMailboxNotUsable) {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
	if got := repo.byKeyFingerprint["edproof:key-1"]; got.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to stay active in free mode, got %s", got.Status)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

func TestResolveAccessByTokenFreeModeDoesNotExpireStaleKeyBoundMailbox(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:    &past,
				AccessToken:  "token-1",
				IMAPHost:     "imap.test.local",
				IMAPPort:     1143,
				IMAPUsername: "mbx_1",
				IMAPPassword: "pass",
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	_, err := service.ResolveIMAPByToken(context.Background(), "token-1")
	if !errors.Is(err, ports.ErrMailboxNotUsable) {
		t.Fatalf("expected ErrMailboxNotUsable, got %v", err)
	}
	if got := repo.byAccessToken["token-1"]; got.Status != domain.MailboxStatusActive {
		t.Fatalf("expected mailbox to stay active in free mode, got %s", got.Status)
	}
	if repo.updated != nil {
		t.Fatalf("expected no repo update in free mode, got %+v", repo.updated)
	}
}

func TestSwitchoverToFreeModeClearsActiveExpiryAndStaysUsable(t *testing.T) {
	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-1",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusActive,
				PaidAt:         ptrTime(time.Now().UTC().Add(-24 * time.Hour)),
				ExpiresAt:      &future,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	result, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("SwitchoverToFreeMode failed: %v", err)
	}
	if result.ActiveCleared != 1 {
		t.Fatalf("expected 1 active mailbox cleared, got %d", result.ActiveCleared)
	}
	mb := repo.byKeyFingerprint["edproof:key-1"]
	if mb.ExpiresAt != nil {
		t.Fatalf("expected cleared ExpiresAt, got %v", mb.ExpiresAt)
	}
	if !mb.Usable() {
		t.Fatalf("expected mailbox to stay usable after switchover (Covers AE3)")
	}
}

func TestSwitchoverToFreeModeConvertsPendingMailboxWithActivationLink(t *testing.T) {
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"pay-1": {
				ID:               "mbx-pending",
				OwnerEmail:       "owner@example.com",
				BillingEmail:     "owner@example.com",
				KeyFingerprint:   "edproof:key-1",
				PaymentSessionID: "pay-1",
				PaymentURL:       "https://pay.example.com/1",
				Status:           domain.MailboxStatusPendingPayment,
			},
		},
	}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, notifier, fakeMailboxTokenGenerator{token: "raw-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)
	service.SetPublicBaseURL("http://test.local")

	result, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("SwitchoverToFreeMode failed: %v", err)
	}
	if result.PendingConverted != 1 {
		t.Fatalf("expected 1 pending converted, got %d", result.PendingConverted)
	}
	if result.PendingEmailsSent != 1 {
		t.Fatalf("expected 1 activation email, got %d", result.PendingEmailsSent)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one activation email sent, got %d", notifier.calls)
	}
	mb := repo.byActivationTokenHash[hashToken("raw-token")]
	if mb == nil {
		t.Fatalf("expected pending mailbox to gain an activation token hash")
	}
	if mb.Status != domain.MailboxStatusPendingPayment {
		t.Fatalf("expected pending status, got %s", mb.Status)
	}
	if mb.ActivationExpiresAt == nil {
		t.Fatalf("expected activation expiry set")
	}

	// Clicking the emailed link activates the mailbox (Covers AE4).
	activated, err := service.ActivateMailboxByActivationToken(context.Background(), "raw-token")
	if err != nil {
		t.Fatalf("activate after switchover failed: %v", err)
	}
	if activated.Mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active after activation, got %s", activated.Mailbox.Status)
	}
	if activated.Mailbox.ExpiresAt != nil {
		t.Fatalf("expected nil ExpiresAt after free activation, got %v", activated.Mailbox.ExpiresAt)
	}
}

func TestSwitchoverToFreeModeSkipsPendingWithValidToken(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	repo := &fakeMailboxRepo{
		byActivationTokenHash: map[string]*domain.Mailbox{
			hashToken("existing-token"): {
				ID:                  "mbx-pending",
				OwnerEmail:          "owner@example.com",
				BillingEmail:        "owner@example.com",
				KeyFingerprint:      "edproof:key-1",
				Status:              domain.MailboxStatusPendingPayment,
				ActivationTokenHash: hashToken("existing-token"),
				ActivationExpiresAt: &future,
			},
		},
	}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, notifier, fakeMailboxTokenGenerator{token: "new-token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	first, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("first SwitchoverToFreeMode failed: %v", err)
	}
	if first.PendingConverted != 0 || first.PendingEmailsSent != 0 {
		t.Fatalf("expected valid-token pending skipped, got %+v", first)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no email for already-converted pending, got %d", notifier.calls)
	}
	if repo.byActivationTokenHash[hashToken("existing-token")].ActivationTokenHash != hashToken("existing-token") {
		t.Fatalf("expected existing token left untouched")
	}

	// Re-running the switchover changes nothing and sends no new emails.
	second, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("second SwitchoverToFreeMode failed: %v", err)
	}
	if second.PendingConverted != 0 || second.PendingEmailsSent != 0 {
		t.Fatalf("expected re-run to be a no-op for pendings, got %+v", second)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected no new emails on re-run, got %d", notifier.calls)
	}
}

func TestSwitchoverToFreeModeClearsAccountExpiry(t *testing.T) {
	accountExpiry := time.Now().UTC().Add(24 * time.Hour)
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acc-1": {
				ID:                    "acc-1",
				OwnerEmail:            "legacy@example.com",
				SubscriptionExpiresAt: &accountExpiry,
			},
		},
	}
	past := time.Now().UTC().Add(-time.Hour)
	repo := &fakeMailboxRepo{
		byAccessToken: map[string]*domain.Mailbox{
			"token-1": {
				ID:           "mbx-1",
				AccountID:    "acc-1",
				Status:       domain.MailboxStatusActive,
				PaidAt:       ptrTime(time.Now().UTC().Add(-2 * time.Hour)),
				ExpiresAt:    &past,
				AccessToken:  "token-1",
				IMAPHost:     "imap.test.local",
				IMAPPort:     1143,
				IMAPUsername: "mbx_1",
				IMAPPassword: "pass",
			},
		},
	}
	service := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	result, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("SwitchoverToFreeMode failed: %v", err)
	}
	if result.AccountsCleared != 1 {
		t.Fatalf("expected 1 account cleared, got %d", result.AccountsCleared)
	}
	if accounts.byID["acc-1"].SubscriptionExpiresAt != nil {
		t.Fatalf("expected account subscription expiry cleared, got %v", accounts.byID["acc-1"].SubscriptionExpiresAt)
	}
	if result.ActiveCleared != 1 {
		t.Fatalf("expected the account-bound active mailbox cleared too, got %d", result.ActiveCleared)
	}

	// The account-bound mailbox survives past its old account deadline.
	if _, err := service.ResolveIMAPByToken(context.Background(), "token-1"); err != nil {
		t.Fatalf("expected resolve to succeed after switchover, got %v", err)
	}
}

func TestSwitchoverToFreeModeLeavesExpiredMailboxesUntouched(t *testing.T) {
	past := time.Now().UTC().Add(-24 * time.Hour)
	repo := &fakeMailboxRepo{
		byKeyFingerprint: map[string]*domain.Mailbox{
			"edproof:key-1": {
				ID:             "mbx-expired",
				KeyFingerprint: "edproof:key-1",
				Status:         domain.MailboxStatusExpired,
				PaidAt:         ptrTime(time.Now().UTC().Add(-48 * time.Hour)),
				ExpiresAt:      &past,
			},
		},
	}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	service.SetFreeMode(true)

	result, err := service.SwitchoverToFreeMode(context.Background())
	if err != nil {
		t.Fatalf("SwitchoverToFreeMode failed: %v", err)
	}
	if result.ActiveCleared != 0 {
		t.Fatalf("expected no active clear for expired mailbox, got %d", result.ActiveCleared)
	}
	if got := repo.byKeyFingerprint["edproof:key-1"]; got.Status != domain.MailboxStatusExpired {
		t.Fatalf("expected expired mailbox untouched, got %s", got.Status)
	}
}

func TestSwitchoverToFreeModeRequiresFreeMode(t *testing.T) {
	repo := &fakeMailboxRepo{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, &fakePaymentGateway{}, &fakeMailboxNotifier{}, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)
	// freeMode defaults to false: switchover must refuse.
	if _, err := service.SwitchoverToFreeMode(context.Background()); !errors.Is(err, ports.ErrSwitchoverRequiresFreeMode) {
		t.Fatalf("expected ErrSwitchoverRequiresFreeMode, got %v", err)
	}
}
