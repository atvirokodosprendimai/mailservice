package domain

import "time"

type MailboxStatus string

const (
	MailboxStatusPendingPayment MailboxStatus = "pending_payment"
	MailboxStatusActive         MailboxStatus = "active"
)

type Mailbox struct {
	ID              string
	AccountID       string
	OwnerEmail      string
	IMAPHost        string
	IMAPPort        int
	IMAPUsername    string
	IMAPPassword    string
	AccessToken     string
	StripeSessionID string
	PaymentURL      string
	Status          MailboxStatus
	PaidAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (m Mailbox) Usable() bool {
	return m.Status == MailboxStatusActive && m.PaidAt != nil
}
