package domain

import "time"

type MailboxStatus string

const (
	MailboxStatusPendingPayment MailboxStatus = "pending_payment"
	MailboxStatusActive         MailboxStatus = "active"
	MailboxStatusExpired        MailboxStatus = "expired"
)

type Mailbox struct {
	ID               string
	AccountID        string
	OwnerEmail       string
	BillingEmail     string
	KeyFingerprint   string
	IMAPHost         string
	IMAPPort         int
	IMAPUsername     string
	IMAPPassword     string
	AccessToken      string
	PaymentSessionID string
	PaymentURL       string
	Status           MailboxStatus
	PaidAt           *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
