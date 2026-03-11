package domain

import (
	"testing"
	"time"
)

func TestAccountSubscriptionActive(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		a    Account
		now  time.Time
		want bool
	}{
		{
			name: "nil expiry",
			a:    Account{},
			now:  now,
			want: false,
		},
		{
			name: "expired subscription",
			a:    Account{SubscriptionExpiresAt: &past},
			now:  now,
			want: false,
		},
		{
			name: "active subscription",
			a:    Account{SubscriptionExpiresAt: &future},
			now:  now,
			want: true,
		},
		{
			name: "exact expiry boundary",
			a:    Account{SubscriptionExpiresAt: &now},
			now:  now,
			want: false, // After is strict (not >=)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.a.SubscriptionActive(tt.now); got != tt.want {
				t.Fatalf("SubscriptionActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
