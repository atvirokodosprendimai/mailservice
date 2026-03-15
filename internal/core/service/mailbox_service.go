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

type GiftCouponConfig struct {
	DiscountID string
	CouponCode string
}

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
	giftCoupon  GiftCouponConfig
}

func NewMailboxService(repo ports.MailboxRepository, accounts ports.AccountRepository, payment ports.PaymentGateway, notifier ports.Notifier, tokenGen ports.TokenGenerator, provisioner ports.MailRuntimeProvisioner, mailReader ports.MailReader, mailDomain string, imapHost string, imapPort int, giftCoupon ...GiftCouponConfig) *MailboxService {
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

	var gc GiftCouponConfig
	if len(giftCoupon) > 0 {
		gc = giftCoupon[0]
		gc.CouponCode = strings.TrimSpace(strings.ToUpper(gc.CouponCode))
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
		giftCoupon:  gc,
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

const giftGrantedMonths = 3

func (s *MailboxService) ClaimMailbox(ctx context.Context, billingEmail string, key ports.VerifiedKey, couponCode string) (*domain.Mailbox, bool, error) {
	billingEmail = strings.TrimSpace(strings.ToLower(billingEmail))
	if billingEmail == "" || !strings.Contains(billingEmail, "@") {
		return nil, false, errors.New("billing_email must be a valid email")
	}
	key.Fingerprint = strings.TrimSpace(strings.ToLower(key.Fingerprint))
	key.Algorithm = strings.TrimSpace(strings.ToLower(key.Algorithm))
	if key.Fingerprint == "" || key.Algorithm == "" {
		return nil, false, ports.ErrInvalidKeyProof
	}

	couponCode = strings.TrimSpace(strings.ToUpper(couponCode))
	discountID, grantedMonths, err := s.validateCoupon(couponCode)
	if err != nil {
		return nil, false, err
	}

	existing, err := s.repo.GetByKeyFingerprint(ctx, key.Fingerprint)
	if err == nil {
		if existing.Usable() {
			return existing, false, nil
		}

		// Per-user dedup: check if this key already redeemed a coupon
		if couponCode != "" && existing.CouponUsed {
			return nil, false, ports.ErrCouponAlreadyUsed
		}

		// If the mailbox already has a pending payment session, return it
		// without creating a new checkout. Creating a new session would
		// invalidate the one the user may have already started paying.
		if existing.Status == domain.MailboxStatusPendingPayment && existing.PaymentSessionID != "" && existing.PaymentURL != "" {
			return existing, false, nil
		}

		paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
			MailboxID:  existing.ID,
			OwnerEmail: billingEmail,
			DiscountID: discountID,
		})
		if err != nil {
			return nil, false, fmt.Errorf("create payment link: %w", err)
		}

		existing.OwnerEmail = billingEmail
		existing.BillingEmail = billingEmail
		existing.PaymentSessionID = paymentLink.SessionID
		existing.PaymentURL = paymentLink.URL
		existing.Status = domain.MailboxStatusPendingPayment
		existing.GrantedMonths = grantedMonths
		if couponCode != "" {
			existing.CouponUsed = true
		}
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
		DiscountID: discountID,
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
		GrantedMonths:    grantedMonths,
		CouponUsed:       couponCode != "",
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
		months := mailbox.GrantedMonths
		if months <= 0 {
			months = 1
		}
		nextExpiry := base.AddDate(0, months, 0)

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

// ReconcileResult holds the outcome of a single mailbox reconciliation attempt.
type ReconcileResult struct {
	MailboxID string `json:"mailbox_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Action    string `json:"action"`
	Error     string `json:"error,omitempty"`
}

// ReconcilePendingPayments checks all mailboxes stuck in pending_payment status
// against the payment gateway. If the gateway reports the checkout as confirmed
// or succeeded, the mailbox is activated via MarkMailboxPaid.
func (s *MailboxService) ReconcilePendingPayments(ctx context.Context) ([]ReconcileResult, error) {
	pending, err := s.repo.ListPendingPayment(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending payments: %w", err)
	}

	results := make([]ReconcileResult, 0, len(pending))
	for _, mb := range pending {
		result := ReconcileResult{
			MailboxID: mb.ID,
			SessionID: mb.PaymentSessionID,
		}

		if mb.PaymentSessionID == "" {
			result.Status = "no_session"
			result.Action = "skipped"
			results = append(results, result)
			continue
		}

		session, err := s.payment.GetPaymentSession(ctx, mb.PaymentSessionID)
		if err != nil {
			result.Status = "error"
			result.Action = "skipped"
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		result.Status = string(session.Status)

		switch session.Status {
		case ports.PaymentSessionStatusConfirmed, ports.PaymentSessionStatusSucceeded:
			if _, err := s.MarkMailboxPaid(ctx, mb.PaymentSessionID); err != nil {
				result.Action = "activate_failed"
				result.Error = err.Error()
			} else {
				result.Action = "activated"
			}
		case ports.PaymentSessionStatusExpired, ports.PaymentSessionStatusFailed:
			result.Action = "no_action"
		default:
			result.Action = "no_action"
		}

		results = append(results, result)
	}

	return results, nil
}

// validateMailboxSubscription checks whether mailbox is currently usable.
// For key-bound mailboxes (empty AccountID) it inspects the mailbox row directly.
// For account-bound mailboxes it loads the account and validates its subscription.
// On success the mailbox status is synchronised in the repository as a side-effect.
func (s *MailboxService) validateMailboxSubscription(ctx context.Context, mailbox *domain.Mailbox, now time.Time) error {
	if strings.TrimSpace(mailbox.AccountID) == "" {
		// Key-bound mailbox: subscription is tracked on the mailbox itself.
		if !mailbox.Usable() {
			if mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
				mailbox.Status = domain.MailboxStatusExpired
				_ = s.repo.Update(ctx, mailbox)
			}
			return ports.ErrMailboxNotUsable
		}
		return nil
	}

	account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
	if err != nil {
		return err
	}

	if !account.SubscriptionActive(now) {
		if mailbox.Status == domain.MailboxStatusActive {
			mailbox.Status = domain.MailboxStatusExpired
			_ = s.repo.Update(ctx, mailbox)
		}
		return ports.ErrMailboxNotUsable
	}

	if mailbox.Status != domain.MailboxStatusActive {
		mailbox.Status = domain.MailboxStatusActive
		mailbox.ExpiresAt = account.SubscriptionExpiresAt
		_ = s.repo.Update(ctx, mailbox)
	}
	return nil
}

func (s *MailboxService) ResolveAccessByToken(ctx context.Context, accessToken string, protocol string) (*ResolveAccessResult, error) {
	if !supportsProtocol(protocol) {
		return nil, errors.New("unsupported protocol")
	}
	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	if err := s.validateMailboxSubscription(ctx, mailbox, time.Now().UTC()); err != nil {
		return nil, err
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

	if err := s.validateMailboxSubscription(ctx, mailbox, time.Now().UTC()); err != nil {
		return nil, err
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

	if err := s.validateMailboxSubscription(ctx, mailbox, time.Now().UTC()); err != nil {
		return nil, err
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

type ReprovisionRequest struct {
	MailboxID      string
	OwnerEmail     string
	KeyFingerprint string
	ExpiresAt      time.Time
}

func (s *MailboxService) ReprovisionMailbox(ctx context.Context, req ReprovisionRequest) (*domain.Mailbox, error) {
	now := time.Now().UTC()

	if existing, err := s.repo.GetByID(ctx, req.MailboxID); err == nil {
		existing.OwnerEmail = req.OwnerEmail
		existing.BillingEmail = req.OwnerEmail
		existing.KeyFingerprint = req.KeyFingerprint
		existing.Status = domain.MailboxStatusActive
		existing.PaidAt = &now
		existing.ExpiresAt = &req.ExpiresAt
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("update mailbox: %w", err)
		}
		if err := s.provisioner.EnsureMailbox(ctx, existing); err != nil {
			return nil, fmt.Errorf("provision mailbox: %w", err)
		}
		return existing, nil
	}

	imapPassword, err := s.tokenGen.NewToken(16)
	if err != nil {
		return nil, fmt.Errorf("generate imap password: %w", err)
	}
	accessToken, err := s.tokenGen.NewToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	mailbox := &domain.Mailbox{
		ID:             req.MailboxID,
		OwnerEmail:     req.OwnerEmail,
		BillingEmail:   req.OwnerEmail,
		KeyFingerprint: req.KeyFingerprint,
		IMAPHost:       s.imapHost,
		IMAPPort:       s.imapPort,
		IMAPUsername:   "mbx_" + strings.ReplaceAll(req.MailboxID[:12], "-", ""),
		IMAPPassword:   imapPassword,
		AccessToken:    accessToken,
		Status:         domain.MailboxStatusActive,
		PaidAt:         &now,
		ExpiresAt:      &req.ExpiresAt,
	}

	if err := s.repo.Create(ctx, mailbox); err != nil {
		return nil, fmt.Errorf("create mailbox: %w", err)
	}
	if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
		return nil, fmt.Errorf("provision mailbox: %w", err)
	}

	return mailbox, nil
}

func (s *MailboxService) MailDomain() string {
	return s.mailDomain
}

func (s *MailboxService) shouldRewriteLegacyIMAPHost(value string) bool {
	host := strings.TrimSpace(strings.ToLower(value))
	if host == "" {
		return true
	}
	return host == "imap.mailservice.local"
}

// validateCoupon checks if the coupon code is valid and returns the Polar discount ID
// and the number of months to grant. Returns ("", 0, nil) when no coupon is provided.
func (s *MailboxService) validateCoupon(couponCode string) (discountID string, grantedMonths int, err error) {
	if couponCode == "" {
		return "", 0, nil
	}
	if s.giftCoupon.CouponCode == "" || s.giftCoupon.DiscountID == "" {
		return "", 0, ports.ErrCouponInvalid
	}
	if couponCode != s.giftCoupon.CouponCode {
		return "", 0, ports.ErrCouponInvalid
	}
	return s.giftCoupon.DiscountID, giftGrantedMonths, nil
}

func supportsProtocol(protocol string) bool {
	return strings.TrimSpace(strings.ToLower(protocol)) == "imap"
}
