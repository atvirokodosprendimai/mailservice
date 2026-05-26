package imapstats

import (
	"context"
	"io"
	"log"
	"os/exec"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func TestIsImapLogin(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "login",
			message: "imap-login: Info: Login: user=foo, method=PLAIN",
			want:    true,
		},
		{
			name:    "disconnected",
			message: "imap-login: Info: Disconnected (no auth attempts in 6 secs)",
			want:    false,
		},
		{
			name:    "master warning",
			message: "dovecot: master: Warning: Killed with signal 15",
			want:    false,
		},
		{
			name:    "empty",
			message: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isImapLogin(tt.message); got != tt.want {
				t.Fatalf("isImapLogin(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestImapLogoutBodyCount(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    int64
		wantOK  bool
	}{
		{
			name:    "typical",
			message: "imap(saint@example.com)<1234><abcd>: Logged out in=235 out=12345 deleted=0 expunged=0 trashed=0 hdr_count=12 hdr_bytes=4567 body_count=8 body_bytes=89012",
			want:    8,
			wantOK:  true,
		},
		{
			name:    "zero count",
			message: "imap(saint@example.com)<1234><abcd>: Logged out in=235 out=12345 deleted=0 expunged=0 trashed=0 hdr_count=12 hdr_bytes=4567 body_count=0 body_bytes=89012",
			want:    0,
			wantOK:  true,
		},
		{
			name:    "missing marker",
			message: "imap-login: Info: Login: user=saint@example.com",
			want:    0,
			wantOK:  false,
		},
		{
			name:    "garbled value",
			message: "Logged out body_count=abc",
			want:    0,
			wantOK:  false,
		},
		{
			name:    "empty",
			message: "",
			want:    0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := imapLogoutBodyCount(tt.message)
			if got != tt.want || gotOK != tt.wantOK {
				t.Fatalf("imapLogoutBodyCount(%q) = (%d, %v), want (%d, %v)", tt.message, got, gotOK, tt.want, tt.wantOK)
			}
		})
	}
}

func TestShipperRunCountsImapLogins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := metrics.NewRegistry(ctx)
	shipper := NewShipper(reg, "dovecot2.service", log.New(io.Discard, "", 0))
	shipper.cmdBuilder = func(unit string) *exec.Cmd {
		if unit != "dovecot2.service" {
			t.Fatalf("unit = %q, want dovecot2.service", unit)
		}
		return exec.Command("sh", "-c", "printf '%s\n' '{\"MESSAGE\":\"imap-login: Info: Login: user=foo\"}' '{\"MESSAGE\":\"imap-login: Info: Disconnected\"}' '{\"MESSAGE\":\"imap-login: Info: Login: user=bar\"}' '{\"MESSAGE\":\"imap(foo@example.com)<1234><abcd>: Logged out in=235 out=12345 body_count=4 body_bytes=89012\"}'")
	}

	done := make(chan error, 1)
	go func() {
		done <- shipper.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancellation")
	}

	if got := reg.Counter("imap_login").Sum24h(); got != 2 {
		t.Fatalf("imap_login counter = %d, want 2", got)
	}
	if got := reg.Counter("imap_message_fetched").Sum24h(); got != 4 {
		t.Fatalf("imap_message_fetched counter = %d, want 4", got)
	}
}
