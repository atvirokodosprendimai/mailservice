package domain

import (
	"testing"
	"time"
)

func TestMailboxUsable(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		m    Mailbox
		want bool
	}{
		{
			name: "active with future expiry",
			m:    Mailbox{Status: MailboxStatusActive, PaidAt: ptrTime(past), ExpiresAt: ptrTime(future)},
			want: true,
		},
		{
			name: "active with past expiry",
			m:    Mailbox{Status: MailboxStatusActive, PaidAt: ptrTime(past), ExpiresAt: ptrTime(past)},
			want: false,
		},
		{
			name: "active nil expiry (no-expiry plan)",
			m:    Mailbox{Status: MailboxStatusActive, PaidAt: ptrTime(past)},
			want: true,
		},
		{
			name: "active nil PaidAt",
			m:    Mailbox{Status: MailboxStatusActive, ExpiresAt: ptrTime(future)},
			want: false,
		},
		{
			name: "pending status with future expiry",
			m:    Mailbox{Status: MailboxStatusPendingPayment, PaidAt: ptrTime(past), ExpiresAt: ptrTime(future)},
			want: false,
		},
		{
			name: "expired status",
			m:    Mailbox{Status: MailboxStatusExpired, PaidAt: ptrTime(past), ExpiresAt: ptrTime(future)},
			want: false,
		},
		{
			name: "zero-value mailbox",
			m:    Mailbox{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.m.Usable(); got != tt.want {
				t.Fatalf("Usable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
