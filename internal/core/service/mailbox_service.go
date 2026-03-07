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
	repo     ports.MailboxRepository
	payment  ports.PaymentGateway
	notifier ports.Notifier
	tokenGen ports.TokenGenerator
}

func NewMailboxService(repo ports.MailboxRepository, payment ports.PaymentGateway, notifier ports.Notifier, tokenGen ports.TokenGenerator) *MailboxService {
	return &MailboxService{
		repo:     repo,
		payment:  payment,
		notifier: notifier,
		tokenGen: tokenGen,
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
	ownerEmail := strings.TrimSpace(strings.ToLower(req.Account.OwnerEmail))

	pending, err := s.repo.GetPendingByAccountID(ctx, req.Account.ID)
	if err == nil {
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
		AccountID:    req.Account.ID,
		OwnerEmail:   ownerEmail,
		IMAPHost:     "imap.mailservice.local",
		IMAPPort:     993,
		IMAPUsername: "mbx_" + strings.ReplaceAll(id[:12], "-", ""),
		IMAPPassword: imapPassword,
		AccessToken:  accessToken,
		Status:       domain.MailboxStatusPendingPayment,
	}

	paymentLink, err := s.payment.CreatePaymentLink(ctx, ports.PaymentLinkRequest{
		MailboxID:  id,
		OwnerEmail: ownerEmail,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create payment link: %w", err)
	}

	mailbox.StripeSessionID = paymentLink.SessionID
	mailbox.PaymentURL = paymentLink.URL

	if err := s.repo.Create(ctx, mailbox); err != nil {
		return nil, false, fmt.Errorf("create mailbox: %w", err)
	}

	if err := s.notifier.SendPaymentLink(ctx, mailbox.OwnerEmail, mailbox.PaymentURL, mailbox.ID); err != nil {
		return nil, false, fmt.Errorf("send payment link: %w", err)
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

	if mailbox.Usable() {
		return mailbox, nil
	}

	now := time.Now().UTC()
	mailbox.Status = domain.MailboxStatusActive
	mailbox.PaidAt = &now
	if err := s.repo.Update(ctx, mailbox); err != nil {
		return nil, err
	}

	return mailbox, nil
}

func (s *MailboxService) ResolveIMAPByToken(ctx context.Context, accessToken string) (*ResolveIMAPResult, error) {
	mailbox, err := s.repo.GetByAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	if !mailbox.Usable() {
		return nil, ports.ErrMailboxNotUsable
	}

	return &ResolveIMAPResult{
		MailboxID: mailbox.ID,
		Host:      mailbox.IMAPHost,
		Port:      mailbox.IMAPPort,
		Username:  mailbox.IMAPUsername,
		Password:  mailbox.IMAPPassword,
	}, nil
}
