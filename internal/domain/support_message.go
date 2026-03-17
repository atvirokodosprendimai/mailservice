package domain

import "time"

type SupportMessage struct {
	ID             string
	MailboxID      string
	KeyFingerprint string
	Subject        string
	Body           string
	CreatedAt      time.Time
}
