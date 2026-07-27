package service

import (
	"context"
	"errors"
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
	payment := &fakePaymentGateway{
		getPaymentSession: func(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
			return &ports.PaymentSession{
				SessionID: sessionID,
				Status:    ports.PaymentSessionStatusOpen,
			}, nil
		},
	}
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
			// Succeeded must stay reusable: ClaimMailbox/MarkMailboxPaid join on
			// PaymentSessionID, so minting a fresh session here would overwrite it
			// and orphan a webhook still in flight for the original session
			// (see KTD1a in the plan).
			name: "succeeded session is reused, PaymentSessionID unchanged",
			getPaymentSession: func(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
				return &ports.PaymentSession{
					SessionID: sessionID,
					Status:    ports.PaymentSessionStatusSucceeded,
				}, nil
			},
			wantSessionID:   "existing-session-123",
			wantPaymentURL:  "https://checkout.polar.sh/existing",
			wantCreateCalls: 0,
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

// TestMarkMailboxPaidAccountLinkedWithGrantedMonths3ExtendsMailboxOnly is a
// regression test for the bug fixed by U7: on the account-linked branch,
// mailbox.ExpiresAt must honor GrantedMonths (a coupon-granted period),
// while account.SubscriptionExpiresAt (which sibling mailboxes on the same
// account share) must keep advancing by exactly one billing period
// regardless — a coupon on one mailbox must not extend sibling mailboxes.
func TestMarkMailboxPaidAccountLinkedWithGrantedMonths3ExtendsMailboxOnly(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeMailboxRepo{
		byStripeSession: map[string]*domain.Mailbox{
			"sess-acct-gift": {
				ID:               "mbx-acct-gift",
				AccountID:        "acct-1",
				PaymentSessionID: "sess-acct-gift",
				Status:           domain.MailboxStatusPendingPayment,
				GrantedMonths:    3,
			},
		},
	}
	accounts := &fakeMailboxAccountRepo{
		byID: map[string]*domain.Account{
			"acct-1": {ID: "acct-1"},
		},
	}
	svc := NewMailboxService(repo, accounts, &fakePaymentGateway{}, &fakeMailboxNotifier{},
		fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{},
		"mail.test.local", "imap.test.local", 1143)

	mailbox, err := svc.MarkMailboxPaid(context.Background(), "sess-acct-gift")
	if err != nil {
		t.Fatalf("MarkMailboxPaid failed: %v", err)
	}
	if mailbox.Status != domain.MailboxStatusActive {
		t.Fatalf("expected active status, got %s", mailbox.Status)
	}

	expectedMailboxExpiry := now.AddDate(0, 3, 0)
	if mailbox.ExpiresAt == nil {
		t.Fatalf("expected ExpiresAt to be set")
	}
	if diff := mailbox.ExpiresAt.Sub(expectedMailboxExpiry); diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expected mailbox.ExpiresAt ~%v, got %v (diff %v)", expectedMailboxExpiry, *mailbox.ExpiresAt, diff)
	}

	expectedAccountExpiry := now.AddDate(0, 1, 0)
	if diff := accounts.lastSubscriptionUpdateExpiresAt.Sub(expectedAccountExpiry); diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expected account.SubscriptionExpiresAt ~%v, got %v (diff %v)", expectedAccountExpiry, accounts.lastSubscriptionUpdateExpiresAt, diff)
	}
	if accounts.lastSubscriptionUpdateAccountID != "acct-1" {
		t.Fatalf("expected subscription update for acct-1, got %q", accounts.lastSubscriptionUpdateAccountID)
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

func (f *fakeMailboxRepo) Create(_ context.Context, mailbox *domain.Mailbox) error {
	f.created = append(f.created, mailbox)
	if f.byKeyFingerprint == nil {
		f.byKeyFingerprint = map[string]*domain.Mailbox{}
	}
	if mailbox.KeyFingerprint != "" {
		f.byKeyFingerprint[mailbox.KeyFingerprint] = mailbox
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

func (f *fakeMailboxRepo) GetBySubscriptionID(_ context.Context, _ string) (*domain.Mailbox, error) {
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
