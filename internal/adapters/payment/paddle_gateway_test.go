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
		if _, hasCheckout := body["checkout"]; hasCheckout {
			t.Fatalf("did not expect checkout in body (checkout.url must come from Paddle's account-level default, not app config): %#v", body)
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

// TestPaddleGatewayCreatePaymentLinkNeverOverridesCheckoutURL guards against
// re-introducing a config-driven checkout.url override on the create request.
// checkout.url must always come from Paddle's account-level default payment
// link (set once, server-side, by U9's provisioning), never from app config —
// otherwise a stale config value could silently diverge from the account's
// true default. PaddleConfig intentionally has no field that could produce a
// "checkout" key here, with or without a discount on the request.
func TestPaddleGatewayCreatePaymentLinkNeverOverridesCheckoutURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if _, hasCheckout := body["checkout"]; hasCheckout {
			t.Fatalf("expected no checkout key in create-transaction request body, got %#v", body)
		}

		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_no_override",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_no_override",
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

	if _, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{
		MailboxID:  "mbx-1",
		OwnerEmail: "billing@example.com",
		DiscountID: "dsc_1",
	}); err != nil {
		t.Fatalf("CreatePaymentLink failed: %v", err)
	}
}

// TestPaddleGatewayCreatePaymentLinkUsesOwnCheckoutPageWhenConfigured asserts
// U6's link-building contract: when CheckoutBaseURL is set, the returned
// PaymentLink points at this app's own checkout page with _ptxn=<txn id>,
// not at Paddle's raw hosted checkout.url — the email must send customers to
// a page this app controls and can degrade gracefully on, not directly to
// Paddle.
func TestPaddleGatewayCreatePaymentLinkUsesOwnCheckoutPageWhenConfigured(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paddleDataResponse(t, w, map[string]any{
			"id":     "txn_own_page",
			"status": "draft",
			"checkout": map[string]any{
				"url": "https://checkout.paddle.com/checkout?_ptxn=txn_own_page",
			},
			"details": map[string]any{
				"totals": map[string]any{"discount": "0"},
			},
		})
	}))
	defer server.Close()

	gateway, err := NewPaddleGateway(PaddleConfig{
		BaseURL:         server.URL,
		APIKey:          "paddle-key",
		PriceID:         "pri_123",
		CheckoutBaseURL: "https://truevipaccess.com/",
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
	want := "https://truevipaccess.com/v1/payments/paddle/checkout?_ptxn=txn_own_page"
	if link.URL != want {
		t.Fatalf("expected own checkout page url %q, got %q", want, link.URL)
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

// TestPaddleGatewayDiscountErrorCodeMapping covers every discount-related
// error code Paddle documents as reachable from POST /transactions, plus the
// fallback for codes we haven't enumerated and the negative case for a
// non-discount error. Codes confirmed against developer.paddle.com/errors/
// on 2026-07-27.
func TestPaddleGatewayDiscountErrorCodeMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    string
		wantErr error
	}{
		{name: "expired discount is exhausted", code: "discount_expired", wantErr: ports.ErrCouponExhausted},
		{name: "usage limit reached is exhausted", code: "discount_usage_limit_exceeded", wantErr: ports.ErrCouponExhausted},
		{name: "unknown discount id is invalid", code: "transaction_discount_not_found", wantErr: ports.ErrCouponInvalid},
		{name: "ineligible for items is invalid", code: "transaction_discount_not_eligible", wantErr: ports.ErrCouponInvalid},
		{name: "currency mismatch is invalid", code: "transaction_invalid_discount_currency", wantErr: ports.ErrCouponInvalid},
		{name: "restricted product inactive is invalid", code: "discount_restricted_product_not_active", wantErr: ports.ErrCouponInvalid},
		{name: "restricted price inactive is invalid", code: "discount_restricted_product_price_not_active", wantErr: ports.ErrCouponInvalid},
		// Guards the fallback: an unenumerated discount-shaped code must still
		// surface as a coupon error, not a generic failure.
		{name: "unenumerated discount code falls back to invalid", code: "discount_some_future_code", wantErr: ports.ErrCouponInvalid},
		// Negative case: a non-discount error must not be mapped to a coupon
		// sentinel, or unrelated failures would masquerade as bad coupons.
		{name: "non-discount error is not a coupon error", code: "transaction_price_not_found", wantErr: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paddleErrorResponse(t, w, http.StatusBadRequest, tc.code)
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
				DiscountID: "dsc_test",
			})
			if err == nil {
				t.Fatalf("expected an error for code %q, got nil", tc.code)
			}
			if tc.wantErr == nil {
				if errors.Is(err, ports.ErrCouponInvalid) || errors.Is(err, ports.ErrCouponExhausted) {
					t.Fatalf("code %q must not map to a coupon sentinel, got %v", tc.code, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("code %q: expected %v, got %v", tc.code, tc.wantErr, err)
			}
		})
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
