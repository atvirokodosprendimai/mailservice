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
	imapHost    string
	imapPort    int
}

func NewMailboxService(repo ports.MailboxRepository, accounts ports.AccountRepository, payment ports.PaymentGateway, notifier ports.Notifier, tokenGen ports.TokenGenerator, provisioner ports.MailRuntimeProvisioner, mailReader ports.MailReader, imapHost string, imapPort int) *MailboxService {
	if strings.TrimSpace(imapHost) == "" {
		imapHost = "mail.local"
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
		imapHost:    imapHost,
		imapPort:    imapPort,
	}
}

type CreateMailboxRequest struct {
	Account *domain.Account
}

type ResolveIMAPResult struct {
	MailboxID string
	Host      string
	Port      int
	Username  string
	Password  string
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

		mailbox.StripeSessionID = paymentLink.SessionID
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

func (s *MailboxService) MarkMailboxPaid(ctx context.Context, stripeSessionID string) (*domain.Mailbox, error) {
	mailbox, err := s.repo.GetByStripeSessionID(ctx, stripeSessionID)
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

func (s *MailboxService) ResolveIMAPByToken(ctx context.Context, accessToken string) (*ResolveIMAPResult, error) {
	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
	if err != nil {
		return nil, err
	}

	if !account.SubscriptionActive(time.Now().UTC()) {
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
	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	return &ResolveIMAPResult{
		MailboxID: mailbox.ID,
		Host:      mailbox.IMAPHost,
		Port:      mailbox.IMAPPort,
		Username:  mailbox.IMAPUsername,
		Password:  mailbox.IMAPPassword,
	}, nil
}

func (s *MailboxService) ListMessagesByToken(ctx context.Context, accessToken string, limit int) ([]ports.IMAPMessage, error) {
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

	account, err := s.accounts.GetByID(ctx, mailbox.AccountID)
	if err != nil {
		return nil, err
	}

	if !account.SubscriptionActive(time.Now().UTC()) {
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

	if s.provisioner != nil {
		if err := s.provisioner.EnsureMailbox(ctx, mailbox); err != nil {
			return nil, err
		}
	}

	if s.mailReader == nil {
		return []ports.IMAPMessage{}, nil
	}

	return s.mailReader.ListMessages(ctx, mailbox.IMAPHost, mailbox.IMAPPort, mailbox.IMAPUsername, mailbox.IMAPPassword, limit)
}
