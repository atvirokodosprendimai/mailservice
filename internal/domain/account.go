package domain

import "time"

type Account struct {
	ID         string
	OwnerEmail string
	APIToken   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
