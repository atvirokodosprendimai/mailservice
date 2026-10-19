package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

func TestOpenCollectiveCheckoutAndPaidOrderMatching(t *testing.T) {
	tag := checkoutTag("mbx-1")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Personal-Token") != "read-only-test-token" {
			t.Errorf("missing personal token header")
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query string = %q", r.URL.RawQuery)
		}
		var payload struct {
			Variables struct {
				Slug string `json:"slug"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Variables.Slug != "truevipaccess" {
			t.Errorf("project slug = %q", payload.Variables.Slug)
		}
		fixture := `{"data":{"account":{"orders":{"nodes":[
{"publicId":"or_1","status":"ACTIVE","frequency":"MONTHLY","lastChargedAt":"2026-10-19T09:00:00Z","amount":{"value":1.15,"currency":"EUR"},"tier":{"slug":"mailbox-monthly"},"tags":["TAG"]},
{"publicId":"or_2","status":"ACTIVE","frequency":"MONTHLY","lastChargedAt":"2026-10-19T09:00:00Z","tier":{"slug":"wrong"},"tags":["TAG"]},
{"publicId":"or_3","status":"PENDING","frequency":"MONTHLY","lastChargedAt":"2026-10-19T09:00:00Z","tier":{"slug":"mailbox-monthly"},"tags":["TAG"]},
{"publicId":"or_4","status":"PAID","frequency":"ONETIME","processedAt":"2026-10-19T09:00:00Z","tier":{"slug":"mailbox-monthly"},"tags":["TAG"]},
{"publicId":"or_5","status":"ACTIVE","frequency":"MONTHLY","lastChargedAt":"2026-10-19T09:00:00Z","tier":null,"tags":["TAG"]},
{"publicId":"or_6","status":"ACTIVE","frequency":"MONTHLY","lastChargedAt":"2026-10-19T09:00:00Z","tier":{"slug":"mailbox-monthly"},"tags":null}
]}}}}`
		_, _ = w.Write([]byte(strings.ReplaceAll(fixture, "TAG", tag)))
	}))
	defer server.Close()
	gateway, err := NewOpenCollectiveGateway(OpenCollectiveConfig{CheckoutURL: "https://opencollective.com/it-uoga/projects/truevipaccess/contribute/mailbox-monthly-106782/checkout", APIURL: server.URL, PersonalToken: "read-only-test-token", ProjectSlug: "truevipaccess", TierSlug: "mailbox-monthly"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := gateway.CreatePaymentLink(context.Background(), ports.PaymentLinkRequest{MailboxID: "mbx-1"})
	if err != nil {
		t.Fatal(err)
	}
	if link.SessionID != tag || len(tag) != 20 {
		t.Fatalf("tag = %q", link.SessionID)
	}
	parsed, _ := url.Parse(link.URL)
	if parsed.Query().Get("tags") != tag {
		t.Fatalf("checkout URL = %q", link.URL)
	}
	if requests != 0 {
		t.Fatal("checkout made a network request")
	}
	charges, err := gateway.ListPaidCharges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(charges) != 1 || charges[0].Tag != tag || !strings.HasPrefix(charges[0].ChargeID, "or_1:") {
		t.Fatalf("charges = %+v", charges)
	}
	session, err := gateway.GetPaymentSession(context.Background(), tag)
	if err != nil || session.Status != ports.PaymentSessionStatusConfirmed {
		t.Fatalf("session = %+v, %v", session, err)
	}
}

func TestOpenCollectiveGraphQLErrorsAreNotEmptyOrders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"denied"}],"data":{"account":null}}`))
	}))
	defer server.Close()
	gateway, err := NewOpenCollectiveGateway(OpenCollectiveConfig{CheckoutURL: "https://opencollective.com/it-uoga/contribute/mailbox-monthly-106782/checkout", APIURL: server.URL, PersonalToken: "token", ProjectSlug: "truevipaccess", TierSlug: "mailbox-monthly"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.ListPaidCharges(context.Background()); err == nil {
		t.Fatal("GraphQL error reported as empty orders")
	}
}
