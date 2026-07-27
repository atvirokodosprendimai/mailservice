package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

func TestPaddleGatewayCreatePaymentLinkIncrementsPaymentLinkCreated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_metrics",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_metrics",
			},
			"details": map[string]any{
				"totals": map[string]any{"discount": "0"},
			},
		})
	}))
	defer server.Close()

	registry := metrics.NewRegistry(context.Background())
	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
		Metrics: registry,
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	if _, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	}); err != nil {
		t.Fatalf("CreatePaymentLink failed: %v", err)
	}

	if got := registry.Counter("payment_link_created").Sum24h(); got != 1 {
		t.Fatalf("payment_link_created = %d, want 1", got)
	}
}

func TestPaddleGatewayCreatePaymentLinkFailureDoesNotIncrementPaymentLinkCreated(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleErrorResponse(t, w, http.StatusInternalServerError, "internal_server_error")
	}))
	defer server.Close()

	registry := metrics.NewRegistry(context.Background())
	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
		Metrics: registry,
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	if _, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	}); err == nil {
		t.Fatalf("expected CreatePaymentLink to fail on 5xx response")
	}

	if got := registry.Counter("payment_link_created").Sum24h(); got != 0 {
		t.Fatalf("payment_link_created = %d, want 0 on failed creation", got)
	}
}

func TestPaddleGatewayGetPaymentSessionIncrementsPaymentSessionLookup(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_lookup",
			"status": "completed",
		})
	}))
	defer server.Close()

	registry := metrics.NewRegistry(context.Background())
	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
		Metrics: registry,
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	if _, err := gateway.GetPaymentSession(context.Background(), "txn_lookup"); err != nil {
		t.Fatalf("GetPaymentSession failed: %v", err)
	}

	if got := registry.Counter("payment_session_lookup").Sum24h(); got != 1 {
		t.Fatalf("payment_session_lookup = %d, want 1", got)
	}
}
