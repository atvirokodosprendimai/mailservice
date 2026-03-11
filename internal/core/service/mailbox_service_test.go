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
	})
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

func TestClaimMailboxCreatesPendingMailboxForNewKey(t *testing.T) {
	repo := &fakeMailboxRepo{}
	payment := &fakePaymentGateway{}
	notifier := &fakeMailboxNotifier{}
	service := NewMailboxService(repo, &fakeMailboxAccountRepo{}, payment, notifier, fakeMailboxTokenGenerator{token: "token"}, &fakeMailRuntimeProvisioner{}, &fakeMailReader{}, "mail.test.local", "imap.test.local", 1143)

	mailbox, created, err := service.ClaimMailbox(context.Background(), "billing@example.com", ports.VerifiedKey{
		Fingerprint: "edproof:key-2",
		Algorithm:   "ed25519",
	})
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
	})
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

type fakeMailboxRepo struct {
	pendingByAccount map[string]*domain.Mailbox
	created          []*domain.Mailbox
	byStripeSession  map[string]*domain.Mailbox
	byAccessToken    map[string]*domain.Mailbox
	byKeyFingerprint map[string]*domain.Mailbox
	updated          *domain.Mailbox
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

type fakePaymentGateway struct {
	calls int
}

func (f *fakePaymentGateway) CreatePaymentLink(_ context.Context, _ ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	f.calls++
	return &ports.PaymentLink{SessionID: "sess-1", URL: "http://pay/1"}, nil
}

func (f *fakePaymentGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
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
