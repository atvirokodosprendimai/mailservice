package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type MailboxService struct {
	repo        ports.MailboxRepository
	accounts    ports.AccountRepository
	payment     ports.PaymentGateway
	notifier    ports.Notifier
	tokenGen    ports.TokenGenerator
	provisioner ports.MailRuntimeProvisioner
	mailReader  ports.MailReader
	mailDomain  string
	imapHost    string
	imapPort    int
}

func NewMailboxService(repo ports.MailboxRepository, accounts ports.AccountRepository, payment ports.PaymentGateway, notifier ports.Notifier, tokenGen ports.TokenGenerator, provisioner ports.MailRuntimeProvisioner, mailReader ports.MailReader, mailDomain string, imapHost string, imapPort int) *MailboxService {
	mailDomain = strings.TrimSpace(strings.ToLower(mailDomain))
	if mailDomain == "" {
		mailDomain = "mail.local"
	}
	trimmedIMAPHost := strings.TrimSpace(strings.ToLower(imapHost))
	if trimmedIMAPHost == "" {
		imapHost = mailDomain
	} else {
		imapHost = trimmedIMAPHost
	}
	if imapPort <= 0 {
		imapPort = 143
	}

	return &MailboxService{
		repo:        repo,
		accounts:    accounts,
		payment:     payment,
		notifier:    notifier,
		tokenGen:    tokenGen,
		provisioner: provisioner,
		mailReader:  mailReader,
		mailDomain:  mailDomain,
		imapHost:    imapHost,
		imapPort:    imapPort,
	}
}

type CreateMailboxRequest struct {
	Account *domain.Account
}

type ResolveIMAPResult struct {
	MailboxID   string
	Host        string
	Port        int
	Username    string
	Password    string
	Email       string
	AccessToken string
}

type ResolveAccessResult = ResolveIMAPResult

func (s *MailboxService) ClaimMailbox(ctx context.Context, billingEmail string, key ports.VerifiedKey) (*domain.Mailbox, bool, error) {
	billingEmail = strings.TrimSpace(strings.ToLower(billingEmail))
	if billingEmail == "" || !strings.Contains(billingEmail, "@") {
		return nil, false, errors.New("billing_email must be a valid email")
	}
	key.Fingerprint = strings.TrimSpace(strings.ToLower(key.Fingerprint))
	key.Algorithm = strings.TrimSpace(strings.ToLower(key.Algorithm))
	if key.Fingerprint == "" || key.Algorithm == "" {
		return nil, false, ports.ErrInvalidKeyProof
	}

	existing, err := s.repo.GetByKeyFingerprint(ctx, key.Fingerprint)
	if err == nil {
		if existing.Usable() {
			return existing, false, nil
		}

		paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
			MailboxID:  existing.ID,
			OwnerEmail: billingEmail,
		})
		if err != nil {
			return nil, false, fmt.Errorf("create payment link: %w", err)
		}

		existing.OwnerEmail = billingEmail
		existing.BillingEmail = billingEmail
		existing.PaymentSessionID = paymentLink.SessionID
		existing.PaymentURL = paymentLink.URL
		existing.Status = domain.MailboxStatusPendingPayment
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, false, fmt.Errorf("update mailbox payment link: %w", err)
		}
		if err := s.notifier.SendPaymentLink(ctx, existing.BillingEmail, existing.PaymentURL, existing.ID); err != nil {
			return nil, false, fmt.Errorf("send payment link: %w", err)
		}
		return existing, false, nil
	}
	if !errors.Is(err, ports.ErrMailboxNotFound) {
		return nil, false, err
	}

	id := uuid.NewString()
	imapPassword, err := s.tokenGen.NewToken(24)
	if err != nil {
		return nil, false, fmt.Errorf("generate imap password: %w", err)
	}
	accessToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, false, fmt.Errorf("generate access token: %w", err)
	}

	paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
		MailboxID:  id,
		OwnerEmail: billingEmail,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create payment link: %w", err)
	}

	mailbox := &domain.Mailbox{
		ID:               id,
		OwnerEmail:       billingEmail,
		BillingEmail:     billingEmail,
		KeyFingerprint:   key.Fingerprint,
		IMAPHost:         s.imapHost,
		IMAPPort:         s.imapPort,
		IMAPUsername:     "mbx_" + strings.ReplaceAll(id[:12], "-", ""),
		IMAPPassword:     imapPassword,
		AccessToken:      accessToken,
		PaymentSessionID: paymentLink.SessionID,
		PaymentURL:       paymentLink.URL,
		Status:           domain.MailboxStatusPendingPayment,
	}

	if err := s.repo.Create(ctx, mailbox); err != nil {
		return nil, false, fmt.Errorf("create mailbox: %w", err)
	}
	if err := s.notifier.SendPaymentLink(ctx, mailbox.BillingEmail, mailbox.PaymentURL, mailbox.ID); err != nil {
		return nil, false, fmt.Errorf("send payment link: %w", err)
	}

	return mailbox, true, nil
}

func (s *MailboxService) CreateMailbox(ctx context.Context, req CreateMailboxRequest) (*domain.Mailbox, bool, error) {
	if req.Account == nil {
		return nil, false, errors.New("account is required")
	}
	now := time.Now().UTC()
	ownerEmail := strings.TrimSpace(strings.ToLower(req.Account.OwnerEmail))
	accountHasActiveSubscription := req.Account.SubscriptionActive(now)

	if !accountHasActiveSubscription {
		pending, err := s.repo.GetPendingByAccountID(ctx, req.Account.ID)
		if err == nil {
			return pending, false, nil
		}
		if !errors.Is(err, ports.ErrMailboxNotFound) {
			return nil, false, err
		}
	}

	id := uuid.NewString()
	imapPassword, err := s.tokenGen.NewToken(24)
	if err != nil {
		return nil, false, fmt.Errorf("generate imap password: %w", err)
	}
	accessToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, false, fmt.Errorf("generate access token: %w", err)
	}

	mailbox := &domain.Mailbox{
		ID:           id,
		AccountID:    req.Account.ID,
		OwnerEmail:   ownerEmail,
		IMAPHost:     s.imapHost,
		IMAPPort:     s.imapPort,
		IMAPUsername: "mbx_" + strings.ReplaceAll(id[:12], "-", ""),
		IMAPPassword: imapPassword,
		AccessToken:  accessToken,
		Status:       domain.MailboxStatusPendingPayment,
	}
	if accountHasActiveSubscription {
		mailbox.Status = domain.MailboxStatusActive
		mailbox.PaidAt = &now
		mailbox.ExpiresAt = req.Account.SubscriptionExpiresAt
	}

	if !accountHasActiveSubscription {
		paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
			MailboxID:  id,
			OwnerEmail: ownerEmail,
		})
		if err != nil {
			return nil, false, fmt.Errorf("create payment link: %w", err)
		}

		mailbox.PaymentSessionID = paymentLink.SessionID
		mailbox.PaymentURL = paymentLink.URL
	}

	if err := s.repo.Create(ctx, mailbox); err != nil {
		return nil, false, fmt.Errorf("create mailbox: %w", err)
	}

	if !accountHasActiveSubscription {
		if err := s.notifier.SendPaymentLink(ctx, mailbox.OwnerEmail, mailbox.PaymentURL, mailbox.ID); err != nil {
			return nil, false, fmt.Errorf("send payment link: %w", err)
		}
	}

	if accountHasActiveSubscription && s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, false, err
		}
	}

	return mailbox, true, nil
}

func (s *MailboxService) GetMailbox(ctx context.Context, id string) (*domain.Mailbox, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *MailboxService) GetMailboxForAccount(ctx context.Context, id string, accountID string) (*domain.Mailbox, error) {
	mailbox, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if mailbox.AccountID != accountID {
		return nil, ports.ErrForbidden
	}
	return mailbox, nil
}

func (s *MailboxService) ListMailboxesForAccount(ctx context.Context, accountID string) ([]domain.Mailbox, error) {
	return s.repo.ListByAccountID(ctx, accountID)
}

func (s *MailboxService) MarkMailboxPaid(ctx context.Context, paymentSessionID string) (*domain.Mailbox, error) {
	mailbox, err := s.repo.GetByPaymentSessionID(ctx, paymentSessionID)
	if err != nil {
		return nil, err
	}

	if mailbox.Status == domain.MailboxStatusActive {
		if s.provisioner != nil {
			if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
				return nil, err
			}
		}
		return mailbox, nil
	}

	now := time.Now().UTC()
	if strings.TrimSpace(mailbox.AccountID) == "" {
		base := now
		if mailbox.ExpiresAt != nil && mailbox.ExpiresAt.After(base) {
			base = *mailbox.ExpiresAt
		}
		nextExpiry := base.AddDate(0, 1, 0)

		mailbox.Status = domain.MailboxStatusActive
		mailbox.PaidAt = &now
		mailbox.ExpiresAt = &nextExpiry
		if err := s.repo.Update(ctx, mailbox); err != nil {
			return nil, err
		}
		if s.provisioner != nil {
			if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
				return nil, err
			}
		}
		return mailbox, nil
	}

	account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
	if err != nil {
		return nil, err
	}
	base := now
	if account.SubscriptionExpiresAt != nil && account.SubscriptionExpiresAt.After(base) {
		base = *account.SubscriptionExpiresAt
	}
	nextExpiry := base.AddDate(0, 1, 0)
	if err := s.accounts.UpdateSubscriptionExpiresAt(ctx, account.ID, nextExpiry); err != nil {
		return nil, err
	}

	mailbox.Status = domain.MailboxStatusActive
	mailbox.PaidAt = &now
	mailbox.ExpiresAt = &nextExpiry
	if err := s.repo.Update(ctx, mailbox); err != nil {
		return nil, err
	}

	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	return mailbox, nil
}

func (s *MailboxService) ResolveAccessByToken(ctx context.Context, accessToken string, protocol string) (*ResolveAccessResult, error) {
	if !supportsProtocol(protocol) {
		return nil, errors.New("unsupported protocol")
	}
	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if strings.TrimSpace(mailbox.AccountID) == "" {
		// Key-bound mailbox: subscription is tracked on the mailbox itself.
		if !mailbox.Usable() {
			if mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}
	} else {
		account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
		if err != nil {
			return nil, err
		}

		if !account.SubscriptionActive(now) {
			if mailbox.Status == domain.MailboxStatusActive {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}

		if mailbox.Status != domain.MailboxStatusActive {
			mailbox.Status = domain.MailboxStatusActive
			mailbox.ExpiresAt = account.SubscriptionExpiresAt
			_ = s.repo.Update(ctx, mailbox)
		}
	}

	if s.shouldRewriteLegacyIMAPHost(mailbox.IMAPHost) || mailbox.IMAPPort <= 0 {
		mailbox.IMAPHost = s.imapHost
		mailbox.IMAPPort = s.imapPort
		_ = s.repo.Update(ctx, mailbox)
	}
	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	return s.resolveAccessResult(mailbox), nil
}

func (s *MailboxService) ResolveIMAPByToken(ctx context.Context, accessToken string) (*ResolveIMAPResult, error) {
	return s.ResolveAccessByToken(ctx, accessToken, "imap")
}

func (s *MailboxService) ResolveAccessByKey(ctx context.Context, key ports.VerifiedKey, protocol string) (*ResolveAccessResult, error) {
	if !supportsProtocol(protocol) {
		return nil, errors.New("unsupported protocol")
	}
	key.Fingerprint = strings.TrimSpace(strings.ToLower(key.Fingerprint))
	key.Algorithm = strings.TrimSpace(strings.ToLower(key.Algorithm))
	if key.Fingerprint == "" || key.Algorithm == "" {
		return nil, ports.ErrInvalidKeyProof
	}

	mailbox, err := s.repo.GetByKeyFingerprint(ctx, key.Fingerprint)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if !mailbox.Usable() {
		if mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
			mailbox.Status = domain.MailboxStatusExpired
			_ = s.repo.Update(ctx, mailbox)
		}
		return nil, ports.ErrMailboxNotUsable
	}

	if s.shouldRewriteLegacyIMAPHost(mailbox.IMAPHost) || mailbox.IMAPPort <= 0 {
		mailbox.IMAPHost = s.imapHost
		mailbox.IMAPPort = s.imapPort
		_ = s.repo.Update(ctx, mailbox)
	}
	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	return s.resolveAccessResult(mailbox), nil
}

func (s *MailboxService) ResolveIMAPByKey(ctx context.Context, key ports.VerifiedKey) (*ResolveIMAPResult, error) {
	return s.ResolveAccessByKey(ctx, key, "imap")
}

func (s *MailboxService) resolveAccessResult(mailbox *domain.Mailbox) *ResolveAccessResult {
	return &ResolveIMAPResult{
		MailboxID:   mailbox.ID,
		Host:        mailbox.IMAPHost,
		Port:        mailbox.IMAPPort,
		Username:    mailbox.IMAPUsername,
		Password:    mailbox.IMAPPassword,
		Email:       mailbox.IMAPUsername + "@" + s.mailDomain,
		AccessToken: mailbox.AccessToken,
	}
}

func (s *MailboxService) ListMessagesByToken(ctx context.Context, accessToken string, limit int, unreadOnly bool, includeBody bool) ([]ports.IMAPMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if strings.TrimSpace(mailbox.AccountID) == "" {
		// Key-bound mailbox: subscription is tracked on the mailbox itself.
		if !mailbox.Usable() {
			if mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}
	} else {
		account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
		if err != nil {
			return nil, err
		}

		if !account.SubscriptionActive(now) {
			if mailbox.Status == domain.MailboxStatusActive {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}

		if mailbox.Status != domain.MailboxStatusActive {
			mailbox.Status = domain.MailboxStatusActive
			mailbox.ExpiresAt = account.SubscriptionExpiresAt
			_ = s.repo.Update(ctx, mailbox)
		}
	}

	if s.shouldRewriteLegacyIMAPHost(mailbox.IMAPHost) || mailbox.IMAPPort <= 0 {
		mailbox.IMAPHost = s.imapHost
		mailbox.IMAPPort = s.imapPort
		_ = s.repo.Update(ctx, mailbox)
	}

	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	if s.mailReader == nil {
		return []ports.IMAPMessage{}, nil
	}

	return s.mailReader.ListMessages(ctx, mailbox.IMAPHost, mailbox.IMAPPort, mailbox.IMAPUsername, mailbox.IMAPPassword, limit, unreadOnly, includeBody)
}

func (s *MailboxService) GetMessageByUIDToken(ctx context.Context, accessToken string, uid uint32, includeBody bool) (*ports.IMAPMessage, error) {
	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if strings.TrimSpace(mailbox.AccountID) == "" {
		// Key-bound mailbox: subscription is tracked on the mailbox itself.
		if !mailbox.Usable() {
			if mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}
	} else {
		account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
		if err != nil {
			return nil, err
		}

		if !account.SubscriptionActive(now) {
			if mailbox.Status == domain.MailboxStatusActive {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return nil, ports.ErrMailboxNotUsable
		}

		if mailbox.Status != domain.MailboxStatusActive {
			mailbox.Status = domain.MailboxStatusActive
			mailbox.ExpiresAt = account.SubscriptionExpiresAt
			_ = s.repo.Update(ctx, mailbox)
		}
	}

	if s.shouldRewriteLegacyIMAPHost(mailbox.IMAPHost) || mailbox.IMAPPort <= 0 {
		mailbox.IMAPHost = s.imapHost
		mailbox.IMAPPort = s.imapPort
		_ = s.repo.Update(ctx, mailbox)
	}

	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	if s.mailReader == nil {
		return nil, ports.ErrMailboxNotFound
	}

	message, err := s.mailReader.GetMessageByUID(ctx, mailbox.IMAPHost, mailbox.IMAPPort, mailbox.IMAPUsername, mailbox.IMAPPassword, uid, includeBody)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, ports.ErrMessageNotFound
	}
	return message, nil
}

func (s *MailboxService) shouldRewriteLegacyIMAPHost(value string) bool {
	host := strings.TrimSpace(strings.ToLower(value))
	if host == "" {
		return true
	}
	return host == "imap.mailservice.local"
}

func supportsProtocol(protocol string) bool {
	return strings.TrimSpace(strings.ToLower(protocol)) == "imap"
}
