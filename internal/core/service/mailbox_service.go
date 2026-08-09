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
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

type GiftCouponConfig struct {
	DiscountID string
	CouponCode string
}

type SupportConfig struct {
	SupportEmail string
	SupportRepo  ports.SupportMessageRepository
}

type MailboxService struct {
	repo          ports.MailboxRepository
	accounts      ports.AccountRepository
	payment       ports.PaymentGateway
	notifier      ports.Notifier
	tokenGen      ports.TokenGenerator
	provisioner   ports.MailRuntimeProvisioner
	mailReader    ports.MailReader
	mailDomain    string
	imapHost      string
	imapPort      int
	giftCoupon    GiftCouponConfig
	support       SupportConfig
	metrics       *metrics.Registry
	freeMode      bool
	publicBaseURL string
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

func (s *MailboxService) SetMetrics(registry *metrics.Registry) {
	s.metrics = registry
}

func (s *MailboxService) SetFreeMode(enabled bool) {
	s.freeMode = enabled
}

// SetPublicBaseURL configures the base URL used to build activation links.
// Called after construction to avoid changing the constructor signature.
func (s *MailboxService) SetPublicBaseURL(baseURL string) {
	s.publicBaseURL = baseURL
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

// activationTokenSize is the byte length of activation tokens (16 bytes = 128 bits).
const activationTokenSize = 16

// activationTokenTTL is how long an activation link stays valid.
const activationTokenTTL = 24 * time.Hour

func (s *MailboxService) ClaimMailbox(ctx context.Context, billingEmail string, key ports.VerifiedKey, couponCode string) (*domain.Mailbox, bool, error) {
	s.metrics.Counter("key_proof_total").Add(1)
	billingEmail = strings.TrimSpace(strings.ToLower(billingEmail))
	if billingEmail == "" || !strings.Contains(billingEmail, "@") {
		return nil, false, errors.New("billing_email must be a valid email")
	}
	key.Fingerprint = strings.TrimSpace(strings.ToLower(key.Fingerprint))
	key.Algorithm = strings.TrimSpace(strings.ToLower(key.Algorithm))
	if key.Fingerprint == "" || key.Algorithm == "" {
		s.metrics.Counter("key_proof_failed").Add(1)
		return nil, false, ports.ErrInvalidKeyProof
	}

	// In free mode, coupons are disabled entirely: treat the code as absent so
	// neither coupon validation nor the coupon-redeemed dedup branch can fire.
	if s.freeMode {
		couponCode = ""
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

		// Free-mode re-claim: regenerate the activation token and re-email the
		// link for any non-usable existing mailbox (pending, expired, or other).
		if s.freeMode {
			existing.OwnerEmail = billingEmail
			existing.BillingEmail = billingEmail
			if err := s.issueActivationLink(ctx, existing, existing.BillingEmail); err != nil {
				return nil, false, err
			}
			return existing, false, nil
		}

		// Per-user dedup: check if this key already redeemed a coupon
		if couponCode != "" && existing.CouponUsed {
			return nil, false, ports.ErrCouponAlreadyUsed
		}

		if existing.Status == domain.MailboxStatusPendingPayment && existing.PaymentSessionID != "" && existing.PaymentURL != "" {
			reusable, err := s.paymentSessionReusable(ctx, existing.PaymentSessionID)
			if err != nil {
				return nil, false, fmt.Errorf("validate payment session: %w", err)
			}
			if reusable {
				return existing, false, nil
			}
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

	mailbox := &domain.Mailbox{
		ID:             id,
		OwnerEmail:     billingEmail,
		BillingEmail:   billingEmail,
		KeyFingerprint: key.Fingerprint,
		IMAPHost:       s.imapHost,
		IMAPPort:       s.imapPort,
		IMAPUsername:   "mbx_" + strings.ReplaceAll(id[:12], "-", ""),
		IMAPPassword:   imapPassword,
		AccessToken:    accessToken,
		Status:         domain.MailboxStatusPendingPayment,
		GrantedMonths:  grantedMonths,
		CouponUsed:     couponCode != "",
	}

	if s.freeMode {
		if err := s.createMailboxWithActivation(ctx, mailbox, mailbox.BillingEmail); err != nil {
			return nil, false, err
		}
		return mailbox, true, nil
	}

	paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
		MailboxID:  id,
		OwnerEmail: billingEmail,
		DiscountID: discountID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create payment link: %w", err)
	}

	mailbox.PaymentSessionID = paymentLink.SessionID
	mailbox.PaymentURL = paymentLink.URL

	if err := s.repo.Create(ctx, mailbox); err != nil {
		return nil, false, fmt.Errorf("create mailbox: %w", err)
	}
	if err := s.notifier.SendPaymentLink(ctx, mailbox.BillingEmail, mailbox.PaymentURL, mailbox.ID); err != nil {
		return nil, false, fmt.Errorf("send payment link: %w", err)
	}

	return mailbox, true, nil
}

func (s *MailboxService) paymentSessionReusable(ctx context.Context, sessionID string) (bool, error) {
	sess, err := s.payment.GetPaymentSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ports.ErrPaymentSessionNotFound) {
			return false, nil
		}
		return false, err
	}
	if sess.Status == ports.PaymentSessionStatusExpired || sess.Status == ports.PaymentSessionStatusFailed {
		return false, nil
	}
	return true, nil
}

// newActivationToken generates a fresh activation token for the mailbox,
// storing only its sha256 hash and expiry. It returns the raw token so the
// caller can build the link; the raw value is never persisted.
func (s *MailboxService) newActivationToken(mailbox *domain.Mailbox) (string, error) {
	raw, err := s.tokenGen.NewToken(activationTokenSize)
	if err != nil {
		return "", err
	}
	mailbox.ActivationTokenHash = hashToken(raw)
	exp := time.Now().UTC().Add(activationTokenTTL)
	mailbox.ActivationExpiresAt = &exp
	mailbox.Status = domain.MailboxStatusPendingPayment
	mailbox.PaymentSessionID = ""
	mailbox.PaymentURL = ""
	return raw, nil
}

// sendActivationLink emails the activation link for the mailbox and records the
// URL on the transient ActivationURL field for the claim response.
func (s *MailboxService) sendActivationLink(ctx context.Context, mailbox *domain.Mailbox, ownerEmail string, rawToken string) error {
	activationURL := s.activationURL(rawToken)
	mailbox.ActivationURL = activationURL
	return s.notifier.SendActivationLink(ctx, ownerEmail, activationURL, mailbox.ID)
}

func (s *MailboxService) activationURL(rawToken string) string {
	return s.publicBaseURL + "/v1/mailboxes/activate?token=" + rawToken
}

// issueActivationLink generates a fresh activation token for an existing
// mailbox, persists it, and emails the activation link.
func (s *MailboxService) issueActivationLink(ctx context.Context, mailbox *domain.Mailbox, ownerEmail string) error {
	raw, err := s.newActivationToken(mailbox)
	if err != nil {
		return fmt.Errorf("generate activation token: %w", err)
	}
	if err := s.repo.Update(ctx, mailbox); err != nil {
		return fmt.Errorf("update mailbox activation link: %w", err)
	}
	return s.sendActivationLink(ctx, mailbox, ownerEmail, raw)
}

// createMailboxWithActivation creates a new mailbox with a fresh activation
// token and emails the activation link.
func (s *MailboxService) createMailboxWithActivation(ctx context.Context, mailbox *domain.Mailbox, ownerEmail string) error {
	raw, err := s.newActivationToken(mailbox)
	if err != nil {
		return fmt.Errorf("generate activation token: %w", err)
	}
	if err := s.repo.Create(ctx, mailbox); err != nil {
		return fmt.Errorf("create mailbox: %w", err)
	}
	return s.sendActivationLink(ctx, mailbox, ownerEmail, raw)
}

func (s *MailboxService) CreateMailbox(ctx context.Context, req CreateMailboxRequest) (*domain.Mailbox, bool, error) {
	if req.Account == nil {
		return nil, false, errors.New("account is required")
	}
	now := time.Now().UTC()
	ownerEmail := strings.TrimSpace(strings.ToLower(req.Account.OwnerEmail))

	if s.freeMode {
		return s.createFreeMailbox(ctx, req.Account, ownerEmail)
	}

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

// createFreeMailbox is the legacy account/token mailbox creation path when free
// mode is on: subscription state is ignored, the mailbox is activation-pending
// with an emailed activation link, and never expires.
func (s *MailboxService) createFreeMailbox(ctx context.Context, account *domain.Account, ownerEmail string) (*domain.Mailbox, bool, error) {
	pending, err := s.repo.GetPendingByAccountID(ctx, account.ID)
	if err == nil {
		if err := s.issueActivationLink(ctx, pending, pending.OwnerEmail); err != nil {
			return nil, false, err
		}
		return pending, false, nil
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

	mailbox := &domain.Mailbox{
		ID:           id,
		AccountID:    account.ID,
		OwnerEmail:   ownerEmail,
		IMAPHost:     s.imapHost,
		IMAPPort:     s.imapPort,
		IMAPUsername: "mbx_" + strings.ReplaceAll(id[:12], "-", ""),
		IMAPPassword: imapPassword,
		AccessToken:  accessToken,
		Status:       domain.MailboxStatusPendingPayment,
	}
	if err := s.createMailboxWithActivation(ctx, mailbox, mailbox.OwnerEmail); err != nil {
		return nil, false, err
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

// ActivationResult describes the outcome of an activation-link click.
type ActivationResult struct {
	Mailbox       *domain.Mailbox
	AlreadyActive bool
}

// ActivateMailboxByActivationToken activates a pending mailbox whose stored
// token hash matches the raw token, mirroring the recovery-code pattern. In
// free mode activation sets PaidAt and leaves ExpiresAt nil so the mailbox
// never expires. An already-active mailbox is idempotent. Invalid, expired, or
// unknown tokens return ErrActivationTokenInvalid without side effects.
func (s *MailboxService) ActivateMailboxByActivationToken(ctx context.Context, rawToken string) (ActivationResult, error) {
	if strings.TrimSpace(rawToken) == "" {
		return ActivationResult{}, ports.ErrActivationTokenInvalid
	}

	mailbox, err := s.repo.GetByActivationTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, ports.ErrMailboxNotFound) {
			return ActivationResult{}, ports.ErrActivationTokenInvalid
		}
		return ActivationResult{}, err
	}

	if mailbox.Status == domain.MailboxStatusActive {
		if s.provisioner != nil {
			if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
				return ActivationResult{}, err
			}
		}
		return ActivationResult{Mailbox: mailbox, AlreadyActive: true}, nil
	}

	if mailbox.Status != domain.MailboxStatusPendingPayment {
		return ActivationResult{}, ports.ErrActivationTokenInvalid
	}

	now := time.Now().UTC()
	if mailbox.ActivationExpiresAt == nil || !mailbox.ActivationExpiresAt.After(now) {
		return ActivationResult{}, ports.ErrActivationTokenInvalid
	}

	mailbox.Status = domain.MailboxStatusActive
	mailbox.PaidAt = &now
	mailbox.ExpiresAt = nil
	if err := s.repo.Update(ctx, mailbox); err != nil {
		return ActivationResult{}, err
	}
	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return ActivationResult{}, err
		}
	}
	return ActivationResult{Mailbox: mailbox}, nil
}

func (s *MailboxService) MarkMailboxPaid(ctx context.Context, paymentSessionID string) (*domain.Mailbox, error) {
	// Free mode makes the payment path inert (KTD4): activation happens through
	// the emailed activation link, never through a payment signal.
	if s.freeMode {
		return nil, nil
	}

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

func (s *MailboxService) RenewMailbox(ctx context.Context, mailboxID string, paidAt time.Time, expiresAt time.Time) error {
	// Free mode makes renewal inert (KTD4): payment signals cannot extend a mailbox.
	if s.freeMode {
		return nil
	}

	mailbox, err := s.repo.GetByID(ctx, mailboxID)
	if err != nil {
		return err
	}

	mailbox.Status = domain.MailboxStatusActive
	mailbox.PaidAt = &paidAt
	mailbox.ExpiresAt = &expiresAt

	return s.repo.Update(ctx, mailbox)
}

func (s *MailboxService) ExpireMailboxByID(ctx context.Context, mailboxID string) error {
	// Free mode makes revocation inert (KTD4): a subscription.revoked webhook
	// cannot expire a mailbox that is now permanent.
	if s.freeMode {
		return nil
	}

	mailbox, err := s.repo.GetByID(ctx, mailboxID)
	if err != nil {
		return err
	}
	mailbox.Status = domain.MailboxStatusExpired
	return s.repo.Update(ctx, mailbox)
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
	// Free mode makes reconciliation inert (KTD4): no payment gateway is consulted.
	if s.freeMode {
		return []ReconcileResult{}, nil
	}

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

// ExpireMailboxes finds all active mailboxes whose ExpiresAt has passed and
// flips their status to expired. It returns the number of mailboxes expired.
// This is designed to be called periodically by a background sweep.
func (s *MailboxService) ExpireMailboxes(ctx context.Context) (int, error) {
	// Free mode makes the expiry sweep inert (KTD4): no row is flipped, including
	// during the deploy-to-switchover window when active mailboxes still carry
	// a non-nil ExpiresAt.
	if s.freeMode {
		return 0, nil
	}

	now := time.Now().UTC()
	expired, err := s.repo.ListActiveExpired(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("list active expired: %w", err)
	}

	count := 0
	for i := range expired {
		mb := &expired[i]
		mb.Status = domain.MailboxStatusExpired
		if err := s.repo.Update(ctx, mb); err != nil {
			return count, fmt.Errorf("expire mailbox %s: %w", mb.ID, err)
		}
		count++
	}
	return count, nil
}

// SwitchoverResult reports what a free-mode switchover changed.
type SwitchoverResult struct {
	ActiveCleared    int `json:"active_cleared"`
	AccountsCleared  int `json:"accounts_cleared"`
	PendingConverted int `json:"pending_converted"`
}

// SwitchoverToFreeMode is the one-off admin transition to the free model
// (KTD6, R6, R7): it clears expiry on all active mailboxes, clears account
// subscription expiries, and converts pending mailboxes to activation-pending
// with a fresh token and re-email. It only runs while free mode is on and is
// idempotent: pending mailboxes that already hold a valid activation token are
// skipped, and already-expired mailboxes are left untouched.
func (s *MailboxService) SwitchoverToFreeMode(ctx context.Context) (SwitchoverResult, error) {
	if !s.freeMode {
		return SwitchoverResult{}, ports.ErrSwitchoverRequiresFreeMode
	}

	var result SwitchoverResult

	cleared, err := s.repo.ClearActiveExpiries(ctx)
	if err != nil {
		return result, fmt.Errorf("clear active mailbox expiries: %w", err)
	}
	result.ActiveCleared = cleared

	accountsCleared, err := s.accounts.ClearSubscriptionExpiresAt(ctx)
	if err != nil {
		return result, fmt.Errorf("clear account subscription expiries: %w", err)
	}
	result.AccountsCleared = accountsCleared

	pending, err := s.repo.ListPendingPayment(ctx)
	if err != nil {
		return result, fmt.Errorf("list pending mailboxes: %w", err)
	}
	now := time.Now().UTC()
	for i := range pending {
		mb := &pending[i]
		if mb.ActivationTokenHash != "" && mb.ActivationExpiresAt != nil && mb.ActivationExpiresAt.After(now) {
			continue // already converted; idempotency
		}
		raw, err := s.newActivationToken(mb)
		if err != nil {
			return result, fmt.Errorf("generate activation token for %s: %w", mb.ID, err)
		}
		if err := s.repo.Update(ctx, mb); err != nil {
			return result, fmt.Errorf("update mailbox %s: %w", mb.ID, err)
		}
		result.PendingConverted++
		ownerEmail := mb.BillingEmail
		if ownerEmail == "" {
			ownerEmail = mb.OwnerEmail
		}
		if err := s.sendActivationLink(ctx, mb, ownerEmail, raw); err != nil {
			return result, fmt.Errorf("send activation link for %s: %w", mb.ID, err)
		}
	}
	return result, nil
}

// validateMailboxSubscription checks whether mailbox is currently usable.
// For key-bound mailboxes (empty AccountID) it inspects the mailbox row directly.
// For account-bound mailboxes it loads the account and validates its subscription.
// On success the mailbox status is synchronised in the repository as a side-effect.
func (s *MailboxService) validateMailboxSubscription(ctx context.Context, mailbox *domain.Mailbox, now time.Time) error {
	if strings.TrimSpace(mailbox.AccountID) == "" {
		// Key-bound mailbox: subscription is tracked on the mailbox itself.
		if !mailbox.Usable() {
			// In free mode the stale active-to-expired flip is inert (KTD4): a
			// past-due mailbox is unusable during the deploy-to-switchover window
			// but must not be permanently marked expired.
			if !s.freeMode && mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
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

	// Free mode removes the account gate entirely (R8). Independently, a mailbox
	// activated under free mode has nil ExpiresAt, which also bypasses the gate
	// after paid mode is re-enabled (grandfather rule).
	if s.freeMode || mailbox.ExpiresAt == nil {
		return nil
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
	s.metrics.Counter("resolve_calls").Add(1)
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
	s.metrics.Counter("resolve_calls").Add(1)
	if !supportsProtocol(protocol) {
		return nil, errors.New("unsupported protocol")
	}
	s.metrics.Counter("key_proof_total").Add(1)
	key.Fingerprint = strings.TrimSpace(strings.ToLower(key.Fingerprint))
	key.Algorithm = strings.TrimSpace(strings.ToLower(key.Algorithm))
	if key.Fingerprint == "" || key.Algorithm == "" {
		s.metrics.Counter("key_proof_failed").Add(1)
		return nil, ports.ErrInvalidKeyProof
	}

	mailbox, err := s.repo.GetByKeyFingerprint(ctx, key.Fingerprint)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if !mailbox.Usable() {
		// In free mode the stale active-to-expired flip is inert (KTD4): a
		// past-due mailbox is unusable during the deploy-to-switchover window
		// but must not be permanently marked expired.
		if !s.freeMode && mailbox.Status == domain.MailboxStatusActive && mailbox.ExpiresAt != nil && !mailbox.ExpiresAt.After(now) {
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

// SetSupportConfig configures the support message subsystem.
// Called after construction to avoid changing the constructor signature.
func (s *MailboxService) SetSupportConfig(cfg SupportConfig) {
	s.support = cfg
}

const supportRateLimit = 3
const supportRateWindow = time.Hour

type SendSupportMessageRequest struct {
	Key     ports.VerifiedKey
	Subject string
	Body    string
}

func (s *MailboxService) SendSupportMessage(ctx context.Context, req SendSupportMessageRequest) error {
	if s.support.SupportEmail == "" || s.support.SupportRepo == nil {
		return errors.New("support not configured")
	}

	mailbox, err := s.repo.GetByKeyFingerprint(ctx, req.Key.Fingerprint)
	if err != nil {
		return err
	}

	count, err := s.support.SupportRepo.CountRecentByFingerprint(ctx, req.Key.Fingerprint, time.Now().UTC().Add(-supportRateWindow))
	if err != nil {
		return fmt.Errorf("check rate limit: %w", err)
	}
	if count >= supportRateLimit {
		return ports.ErrRateLimitReached
	}

	msg := &domain.SupportMessage{
		ID:             uuid.New().String(),
		MailboxID:      mailbox.ID,
		KeyFingerprint: req.Key.Fingerprint,
		Subject:        req.Subject,
		Body:           req.Body,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.support.SupportRepo.Create(ctx, msg); err != nil {
		return fmt.Errorf("persist support message: %w", err)
	}

	agentEmail := fmt.Sprintf("%s@%s", mailbox.IMAPUsername, s.mailDomain)
	params := ports.SupportMessageParams{
		ToEmail:     s.support.SupportEmail,
		ReplyTo:     agentEmail,
		MailboxID:   mailbox.ID,
		Fingerprint: req.Key.Fingerprint,
		Status:      string(mailbox.Status),
		OwnerEmail:  mailbox.OwnerEmail,
		Subject:     req.Subject,
		Body:        req.Body,
	}
	if err := s.notifier.SendSupportMessage(ctx, params); err != nil {
		return fmt.Errorf("send support message: %w", err)
	}

	return nil
}
