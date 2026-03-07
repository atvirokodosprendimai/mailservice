package domain

import "time"

type Account struct {
	ID                    string
	OwnerEmail            string
	APIToken              string
	SubscriptionExpiresAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (a Account) SubscriptionActive(now time.Time) bool {
	if a.SubscriptionExpiresAt == nil {
		return false
	}
	return a.SubscriptionExpiresAt.After(now.UTC())
}
