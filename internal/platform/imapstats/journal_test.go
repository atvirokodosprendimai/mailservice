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

func TestShipperRunCountsImapLogins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := metrics.NewRegistry(ctx)
	shipper := NewShipper(reg, "dovecot2.service", log.New(io.Discard, "", 0))
	shipper.cmdBuilder = func(unit string) *exec.Cmd {
		if unit != "dovecot2.service" {
			t.Fatalf("unit = %q, want dovecot2.service", unit)
		}
		return exec.Command("sh", "-c", "printf '%s\n' '{\"MESSAGE\":\"imap-login: Info: Login: user=foo\"}' '{\"MESSAGE\":\"imap-login: Info: Disconnected\"}' '{\"MESSAGE\":\"imap-login: Info: Login: user=bar\"}'")
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
}
