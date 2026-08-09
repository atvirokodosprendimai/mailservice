package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
	"github.com/google/uuid"
)

const activationTokenSize = 16

const activationTokenTTL = 24 * time.Hour

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
	if strings.TrimSpace(s.publicBaseURL) == "" {
		return errors.New("public base URL not configured; cannot build activation link")
	}
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

// ActivationResult describes the outcome of an activation-link click.
type ActivationResult struct {
	Mailbox       *domain.Mailbox
	AlreadyActive bool
}

// SwitchoverResult reports what a free-mode switchover changed.
type SwitchoverResult struct {
	ActiveCleared    int `json:"active_cleared"`
	AccountsCleared  int `json:"accounts_cleared"`
	PendingConverted int `json:"pending_converted"`
}
