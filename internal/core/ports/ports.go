package ports

import (
	"context"
	"errors"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

var (
	ErrMailboxNotFound  = errors.New("mailbox not found")
	ErrMailboxNotUsable = errors.New("mailbox not usable")
	ErrAccountNotFound  = errors.New("account not found")
	ErrForbidden        = errors.New("forbidden")
	ErrAccountExists    = errors.New("account already exists")
	ErrRecoveryNotFound = errors.New("recovery not found")
	ErrRecoveryInvalid  = errors.New("recovery code invalid")
	ErrRecoveryExpired  = errors.New("recovery code expired")
	ErrRateLimitReached = errors.New("rate limit reached")
	ErrRefreshNotFound  = errors.New("refresh token not found")
	ErrRefreshExpired   = errors.New("refresh token expired")
	ErrMessageNotFound  = errors.New("message not found")
	ErrInvalidKeyProof  = errors.New("invalid key proof")
)

type MailboxRepository interface {
	Create(ctx context.Context, mailbox *domain.Mailbox) error
	Update(ctx context.Context, mailbox *domain.Mailbox) error
	GetByID(ctx context.Context, id string) (*domain.Mailbox, error)
	ListByAccountID(ctx context.Context, accountID string) ([]domain.Mailbox, error)
	GetPendingByAccountID(ctx context.Context, accountID string) (*domain.Mailbox, error)
	GetByPaymentSessionID(ctx context.Context, sessionID string) (*domain.Mailbox, error)
	GetByAccessToken(ctx context.Context, accessToken string) (*domain.Mailbox, error)
	GetByKeyFingerprint(ctx context.Context, keyFingerprint string) (*domain.Mailbox, error)
}

type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, accountID string) (*domain.Account, error)
	GetByOwnerEmail(ctx context.Context, ownerEmail string) (*domain.Account, error)
	GetByAPIToken(ctx context.Context, apiToken string) (*domain.Account, error)
	UpdateAPIToken(ctx context.Context, accountID string, apiToken string) error
	UpdateSubscriptionExpiresAt(ctx context.Context, accountID string, expiresAt time.Time) error
}

type AccountRecoveryRepository interface {
	Create(ctx context.Context, recovery *domain.AccountRecovery) error
	DeleteActiveByAccountID(ctx context.Context, accountID string) error
	GetLatestByAccountID(ctx context.Context, accountID string) (*domain.AccountRecovery, error)
	GetLatestActiveByAccountID(ctx context.Context, accountID string) (*domain.AccountRecovery, error)
	GetActiveByCodeHash(ctx context.Context, codeHash string) (*domain.AccountRecovery, error)
	MarkUsed(ctx context.Context, recoveryID string, usedAt time.Time) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	MarkUsed(ctx context.Context, tokenID string, usedAt time.Time) error
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
	SendRecoveryLink(ctx context.Context, ownerEmail string, recoveryURL string) error
}

type TokenGenerator interface {
	NewToken(size int) (string, error)
}

type VerifiedKey struct {
	Fingerprint string
	Algorithm   string
}

type KeyProofVerifier interface {
	Verify(ctx context.Context, rawProof string) (*VerifiedKey, error)
}

type MailRuntimeProvisioner interface {
	EnsureMailbox(ctx context.Context, mailbox *domain.Mailbox) error
}

type IMAPMessage struct {
	UID     uint32
	Subject string
	From    string
	Date    time.Time
	Body    string
}

type MailReader interface {
	ListMessages(ctx context.Context, host string, port int, username string, password string, limit int, unreadOnly bool, includeBody bool) ([]IMAPMessage, error)
	GetMessageByUID(ctx context.Context, host string, port int, username string, password string, uid uint32, includeBody bool) (*IMAPMessage, error)
}
