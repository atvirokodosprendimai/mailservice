package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stripe/stripe-go/v83"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

// TestStripeGatewayGetPaymentSessionStatusMapping locks down the Stripe
// payment_status -> ports.PaymentSessionStatus mapping that
// paymentSessionReusable (internal/core/service/mailbox_service.go) relies on
// to decide whether a Stripe-backed pending mailbox's checkout session can be
// reused. paymentSessionReusable rejects reuse only for PaymentSessionStatusFailed
// (plus a 404) — a succeeded session stays reusable so ClaimMailbox never
// overwrites PaymentSessionID and orphans an in-flight webhook (see KTD1a).
// If Stripe's mapping ever drifted so that a completed session started
// reporting Failed, that would incorrectly start regenerating live Stripe
// checkout sessions; this test would catch it.
func TestStripeGatewayGetPaymentSessionStatusMapping(t *testing.T) {
	tests := []struct {
		name          string
		paymentStatus string
		wantStatus    ports.PaymentSessionStatus
		wantReusable  bool
	}{
		{
			name:          "unpaid session stays open and reusable",
			paymentStatus: "unpaid",
			wantStatus:    ports.PaymentSessionStatusOpen,
			wantReusable:  true,
		},
		{
			name:          "paid session is succeeded and stays reusable",
			paymentStatus: "paid",
			wantStatus:    ports.PaymentSessionStatusSucceeded,
			wantReusable:  true,
		},
		{
			name:          "no_payment_required session is confirmed and stays reusable",
			paymentStatus: "no_payment_required",
			wantStatus:    ports.PaymentSessionStatusConfirmed,
			wantReusable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/v1/checkout/sessions/sess_123" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":             "sess_123",
					"url":            "https://checkout.stripe.com/sess_123",
					"payment_status": tt.paymentStatus,
				})
			}))
			defer server.Close()

			originalBackend := stripe.GetBackend(stripe.APIBackend)
			originalKey := stripe.Key
			stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
				URL: stripe.String(server.URL),
			}))
			defer func() {
				stripe.SetBackend(stripe.APIBackend, originalBackend)
				stripe.Key = originalKey
			}()

			gateway := NewStripeGateway(StripeConfig{SecretKey: "sk_test_123"})

			session, err := gateway.GetPaymentSession(context.Background(), "sess_123")
			if err != nil {
				t.Fatalf("GetPaymentSession failed: %v", err)
			}
			if session.Status != tt.wantStatus {
				t.Fatalf("unexpected status: got %q, want %q", session.Status, tt.wantStatus)
			}

			reusable := session.Status != ports.PaymentSessionStatusFailed
			if reusable != tt.wantReusable {
				t.Fatalf("unexpected reusability for status %q: got %v, want %v", session.Status, reusable, tt.wantReusable)
			}
		})
	}
}
