package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

func paddleDataResponse(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func paddleErrorResponse(t *testing.T, w http.ResponseWriter, status int, code string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":   "request_error",
			"code":   code,
			"detail": code,
		},
	})
}

func TestPaddleGatewayCreatePaymentLink(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/transactions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer paddle-key" {
			t.Fatalf("unexpected auth header: %q", auth)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		items, ok := body["items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("unexpected items: %#v", body["items"])
		}
		item, ok := items[0].(map[string]any)
		if !ok || item["price_id"] != "pri_123" {
			t.Fatalf("unexpected item: %#v", items[0])
		}
		customData, ok := body["custom_data"].(map[string]any)
		if !ok || customData["mailbox_id"] != "mbx-1" || customData["owner_email"] != "billing@example.com" {
			t.Fatalf("unexpected custom_data: %#v", body["custom_data"])
		}
		if _, hasDiscount := body["discount_id"]; hasDiscount {
			t.Fatalf("did not expect discount_id in body: %#v", body)
		}

		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_123",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_123",
			},
			"details": map[string]any{
				"totals": map[string]any{"discount": "0"},
			},
		})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	})
	if err != nil {
		t.Fatalf("CreatePaymentLink failed: %v", err)
	}
	if link.SessionID != "txn_123" {
		t.Fatalf("unexpected session id: %q", link.SessionID)
	}
	if link.URL != "https://checkout.paddle.com/checkout?_ptxn=txn_123" {
		t.Fatalf("unexpected url: %q", link.URL)
	}
}

func TestPaddleGatewayCreatePaymentLinkWithDiscount(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["discount_id"] != "dsc_1" {
			t.Fatalf("expected discount_id dsc_1, got %#v", body["discount_id"])
		}

		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_disc",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_disc",
			},
			"details": map[string]any{
				"totals": map[string]any{"discount": "100"},
			},
		})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
		DiscountID: "dsc_1",
	})
	if err != nil {
		t.Fatalf("CreatePaymentLink failed: %v", err)
	}
	if link.SessionID != "txn_disc" {
		t.Fatalf("unexpected session id: %q", link.SessionID)
	}
}

func TestPaddleGatewayCreatePaymentLinkMissingCheckoutURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_no_url",
			"status": "draft",
		})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	})
	if !errors.Is(err, errPaddleMissingCheckoutURL) {
		t.Fatalf("expected errPaddleMissingCheckoutURL, got link=%v err=%v", link, err)
	}
	if link != nil {
		t.Fatalf("expected nil link on missing checkout url, got %#v", link)
	}
}

func TestPaddleGatewayCreatePaymentLinkDiscountRejected(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleErrorResponse(t, w, http.StatusBadRequest, "transaction_discount_not_found")
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
		DiscountID: "dsc_bad",
	})
	if !errors.Is(err, ports.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid, got link=%v err=%v", link, err)
	}
}

func TestPaddleGatewayCreatePaymentLinkDiscountExpired(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleErrorResponse(t, w, http.StatusBadRequest, "discount_expired")
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	_, err = gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
		DiscountID: "dsc_expired",
	})
	if !errors.Is(err, ports.ErrCouponExhausted) {
		t.Fatalf("expected ErrCouponExhausted, got %v", err)
	}
}

func TestPaddleGatewayCreatePaymentLinkDiscountSilentlyNotApplied(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_silent",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_silent",
			},
			"details": map[string]any{
				"totals": map[string]any{"discount": "0"},
			},
		})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
		DiscountID: "dsc_silent",
	})
	if !errors.Is(err, ports.ErrCouponInvalid) {
		t.Fatalf("expected ErrCouponInvalid for silently-unapplied discount, got link=%v err=%v", link, err)
	}
}

func TestPaddleGatewayCreatePaymentLink5xx(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleErrorResponse(t, w, http.StatusInternalServerError, "internal_server_error")
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	})
	if err == nil {
		t.Fatalf("expected error for 5xx response, got link=%v", link)
	}
	if link != nil {
		t.Fatalf("expected nil link on 5xx, got %#v", link)
	}
}

func TestPaddleGatewayCreatePaymentLinkTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		paddleDataResponse(t, w, map[string]any{"id": "txn_slow"})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
		Client:  &http.Client{Timeout: 5 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
	})
	if err == nil {
		t.Fatalf("expected timeout error, got link=%v", link)
	}
	if link != nil {
		t.Fatalf("expected nil link on timeout, got %#v", link)
	}
}

func TestPaddleGatewayGetPaymentSessionStatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		paddleStatus string
		want         ports.PaymentSessionStatus
	}{
		{"draft", ports.PaymentSessionStatusOpen},
		{"ready", ports.PaymentSessionStatusOpen},
		{"billed", ports.PaymentSessionStatusSucceeded},
		{"paid", ports.PaymentSessionStatusSucceeded},
		{"completed", ports.PaymentSessionStatusSucceeded},
		{"canceled", ports.PaymentSessionStatusFailed},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.paddleStatus, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Fatalf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/transactions/txn_123" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				paddleDataResponse(t, w, map[string]any{
					"id":     "txn_123",
					"status": tc.paddleStatus,
					"checkout": map[string]any{
						"url": "https://checkout.paddle.com/checkout?_ptxn=txn_123",
					},
				})
			}))
			defer server.Close()

			gateway, err := NewPaddleGateway(PaddleConfig{
				BaseURL: server.URL,
				APIKey:  "paddle-key",
				PriceID: "pri_123",
			})
			if err != nil {
				t.Fatalf("NewPaddleGateway failed: %v", err)
			}

			session, err := gateway.GetPaymentSession(context.Background(), "txn_123")
			if err != nil {
				t.Fatalf("GetPaymentSession failed: %v", err)
			}
			if session.Status != tc.want {
				t.Fatalf("unexpected status: got %q, want %q", session.Status, tc.want)
			}
		})
	}
}

func TestPaddleGatewayGetPaymentSessionNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transactions/missing_txn" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		paddleErrorResponse(t, w, http.StatusNotFound, "entity_not_found")
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL: server.URL,
		APIKey:  "paddle-key",
		PriceID: "pri_123",
	})
	if err != nil {
		t.Fatalf("NewPaddleGateway failed: %v", err)
	}

	session, err := gateway.GetPaymentSession(context.Background(), "missing_txn")
	if !errors.Is(err, ports.ErrPaymentSessionNotFound) {
		t.Fatalf("expected ErrPaymentSessionNotFound, got session=%v err=%v", session, err)
	}
}
