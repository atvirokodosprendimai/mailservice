package domain

import "time"

type MailboxStatus string

const (
	MailboxStatusPendingPayment MailboxStatus = "pending_payment"
	MailboxStatusActive         MailboxStatus = "active"
	MailboxStatusExpired        MailboxStatus = "expired"
)

type Mailbox struct {
	ID                  string
	AccountID           string
	OwnerEmail          string
	BillingEmail        string
	KeyFingerprint      string
	IMAPHost            string
	IMAPPort            int
	IMAPUsername        string
	IMAPPassword        string
	AccessToken         string
	PaymentSessionID    string
	PaymentURL          string
	ActivationTokenHash string
	ActivationExpiresAt *time.Time
	// ActivationURL is a transient in-memory field used only for claim responses
	// and emails in free mode. It is never persisted: the repository model does
	// not map it, and toModel/toDomain ignore it.
	ActivationURL string
	Status        MailboxStatus
	GrantedMonths int
	CouponUsed    bool
	PaidAt        *time.Time
	ExpiresAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (m Mailbox) Usable() bool {
	if m.Status != MailboxStatusActive || m.PaidAt == nil {
		return false
	}
	if m.ExpiresAt == nil {
		return true
	}
	return m.ExpiresAt.After(time.Now().UTC())
}
