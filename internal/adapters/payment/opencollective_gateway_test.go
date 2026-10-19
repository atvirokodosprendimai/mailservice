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
{"publicId":"or_1","frequency":"MONTHLY","amount":{"value":1.15,"currency":"EUR"},"tier":{"slug":"mailbox-monthly"},"tags":["TAG"],"transactions":[
{"publicId":"tx_1","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-09-19T09:00:00Z","isRefunded":false},
{"publicId":"tx_2","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z","isRefunded":false},
{"publicId":"tx_tip","type":"CREDIT","kind":"PLATFORM_TIP","createdAt":"2026-10-19T09:00:00Z","isRefunded":false},
{"publicId":"tx_unknown","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z","isRefunded":null},
{"publicId":"tx_refund","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-08-19T09:00:00Z","isRefunded":true}]},
{"publicId":"or_2","frequency":"MONTHLY","tier":{"slug":"wrong"},"tags":["TAG"],"transactions":[{"publicId":"tx_3","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z"}]},
{"publicId":"or_3","frequency":"MONTHLY","tier":{"slug":"mailbox-monthly"},"tags":["TAG"],"transactions":[]},
{"publicId":"or_4","frequency":"ONETIME","tier":{"slug":"mailbox-monthly"},"tags":["TAG"],"transactions":[{"publicId":"tx_4","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z"}]},
{"publicId":"or_5","frequency":"MONTHLY","tier":null,"tags":["TAG"],"transactions":[{"publicId":"tx_5","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z"}]},
{"publicId":"or_6","frequency":"MONTHLY","tier":{"slug":"mailbox-monthly"},"tags":null,"transactions":[{"publicId":"tx_6","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z"}]}
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
	if len(charges) != 2 || charges[0].Tag != tag || charges[0].ChargeID != "tx_1" || charges[1].ChargeID != "tx_2" {
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

// One €1 order must never pay for more than one mailbox: an order carrying
// several tva- tags is ambiguous and credits none of them.
func TestOpenCollectiveOrderWithSeveralMailboxTagsCreditsNone(t *testing.T) {
	a, b := checkoutTag("mbx-a"), checkoutTag("mbx-b")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture := `{"data":{"account":{"orders":{"nodes":[
{"publicId":"or_1","frequency":"MONTHLY","tier":{"slug":"mailbox-monthly"},"tags":["A","B","unrelated"],"transactions":[
{"publicId":"tx_1","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T09:00:00Z","isRefunded":false}]},
{"publicId":"or_2","frequency":"MONTHLY","tier":{"slug":"mailbox-monthly"},"tags":["A","unrelated"],"transactions":[
{"publicId":"tx_2","type":"CREDIT","kind":"CONTRIBUTION","createdAt":"2026-10-19T10:00:00Z","isRefunded":false}]}
]}}}}`
		_, _ = w.Write([]byte(strings.NewReplacer("\"A\"", "\""+a+"\"", "\"B\"", "\""+b+"\"").Replace(fixture)))
	}))
	defer server.Close()
	gateway, err := NewOpenCollectiveGateway(OpenCollectiveConfig{CheckoutURL: "https://opencollective.com/it-uoga/projects/truevipaccess/contribute/mailbox-monthly-106782/checkout", APIURL: server.URL, PersonalToken: "t", ProjectSlug: "truevipaccess", TierSlug: "mailbox-monthly"})
	if err != nil {
		t.Fatal(err)
	}
	charges, err := gateway.ListPaidCharges(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(charges) != 1 || charges[0].Tag != a || charges[0].ChargeID != "tx_2" {
		t.Fatalf("charges = %+v, want only tx_2 for %s", charges, a)
	}
}
