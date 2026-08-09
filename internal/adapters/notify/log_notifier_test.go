package notify

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestLogNotifierSendActivationLink(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	n := NewLogNotifier(logger)

	err := n.SendActivationLink(context.Background(), "user@example.com", "https://mail.example/activate?token=abc123", "mbx-1")
	if err != nil {
		t.Fatalf("SendActivationLink failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "activation_url=https://mail.example/activate?token=abc123") {
		t.Fatalf("expected activation URL in log line, got: %s", out)
	}
	if !strings.Contains(out, "mailbox=mbx-1") {
		t.Fatalf("expected mailbox id in log line, got: %s", out)
	}
	if !strings.Contains(out, "owner=user@example.com") {
		t.Fatalf("expected owner in log line, got: %s", out)
	}
}
