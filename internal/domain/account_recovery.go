package domain

import "time"

type AccountRecovery struct {
	ID        string
	AccountID string
	CodeHash  string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
