package ports

import (
	"context"
	"errors"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

var (
	ErrMailboxNotFound  = errors.New("mailbox not found")
	ErrMailboxNotUsable = errors.New("mailbox not usable")
	ErrAccountNotFound  = errors.New("account not found")
	ErrForbidden        = errors.New("forbidden")
)

type MailboxRepository interface {
	Create(ctx context.Context, mailbox *domain.Mailbox) error
	Update(ctx context.Context, mailbox *domain.Mailbox) error
	GetByID(ctx context.Context, id string) (*domain.Mailbox, error)
	ListByAccountID(ctx context.Context, accountID string) ([]domain.Mailbox, error)
	GetPendingByAccountID(ctx context.Context, accountID string) (*domain.Mailbox, error)
	GetByStripeSessionID(ctx context.Context, sessionID string) (*domain.Mailbox, error)
	GetByAccessToken(ctx context.Context, accessToken string) (*domain.Mailbox, error)
}

type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByOwnerEmail(ctx context.Context, ownerEmail string) (*domain.Account, error)
	GetByAPIToken(ctx context.Context, apiToken string) (*domain.Account, error)
}

type PaymentLinkRequest struct {
	MailboxID  string
	OwnerEmail string
}

type PaymentLink struct {
	SessionID string
	URL       string
}

type PaymentGateway interface {
	CreatePaymentLink(ctx context.Context, req PaymentLinkRequest) (*PaymentLink, error)
}

type Notifier interface {
	SendPaymentLink(ctx context.Context, ownerEmail string, paymentURL string, mailboxID string) error
}

type TokenGenerator interface {
	NewToken(size int) (string, error)
}
