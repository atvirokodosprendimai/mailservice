package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

func TestPolarGatewayCreatePaymentLink(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/checkouts/custom/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer polar-token" {
			t.Fatalf("unexpected auth header: %q", auth)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["product_price_id"] != "price_123" {
			t.Fatalf("unexpected product_price_id: %#v", body["product_price_id"])
		}
		if body["customer_email"] != "billing@example.com" {
			t.Fatalf("unexpected customer_email: %#v", body["customer_email"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "polar_123",
			"url":    "https://polar.sh/checkout/polar_123",
			"status": "open",
		})
	}))
	defer server.Close()

	gateway := NewPolarGateway(PolarConfig{
		ServerURL:  server.URL,
		Token:      "polar-token",
		PriceID:    "price_123",
		SuccessURL: "http://localhost/success?checkout_id={CHECKOUT_ID}",
		ReturnURL:  "http://localhost",
	})

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	})
	if err != nil {
		t.Fatalf("CreatePaymentLink failed: %v", err)
	}
	if link.SessionID != "polar_123" {
		t.Fatalf("unexpected session id: %q", link.SessionID)
	}
	if link.URL != "https://polar.sh/checkout/polar_123" {
		t.Fatalf("unexpected url: %q", link.URL)
	}
}

func TestPolarGatewayGetPaymentSession(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/checkouts/polar_123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "polar_123",
			"url":    "https://polar.sh/checkout/polar_123",
			"status": "succeeded",
		})
	}))
	defer server.Close()

	gateway := NewPolarGateway(PolarConfig{
		ServerURL: server.URL,
		Token:     "polar-token",
		PriceID:   "price_123",
	})

	session, err := gateway.GetPaymentSession(context.Background(), "polar_123")
	if err != nil {
		t.Fatalf("GetPaymentSession failed: %v", err)
	}
	if session.Status != ports.PaymentSessionStatusSucceeded {
		t.Fatalf("unexpected status: %q", session.Status)
	}
}
