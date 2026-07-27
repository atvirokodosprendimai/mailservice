package httpapi

import (
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
)

func legalPagesTestHandler() *Handler {
	return NewHandler(Config{
		BuildNumber: "1234",
		CacheBuster: "1234-abcd",
		Logger:      log.New(io.Discard, "", 0),
	})
}

func TestLegalPagesRequireNoAuthAndReturn200(t *testing.T) {
	handler := legalPagesTestHandler()

	for _, path := range []string{"/terms", "/refund-policy", "/privacy", "/contact"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()

		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Fatalf("%s: expected status 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("%s: expected text/html content type, got %q", path, got)
		}
	}
}

func TestLegalPagesSetStrictCSPWithNoScriptSrc(t *testing.T) {
	handler := legalPagesTestHandler()

	for _, path := range []string{"/terms", "/refund-policy", "/privacy", "/contact"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()

		handler.Routes().ServeHTTP(rec, req)

		csp := rec.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatalf("%s: expected a Content-Security-Policy header", path)
		}
		if strings.Contains(csp, "script-src") {
			t.Fatalf("%s: expected no script-src in CSP (pages have no JS), got %q", path, csp)
		}
		if !strings.Contains(csp, "default-src 'none'") {
			t.Fatalf("%s: expected default-src 'none' in CSP, got %q", path, csp)
		}
	}
}

func TestLegalPagesCrossLinkToEachOtherAndHome(t *testing.T) {
	handler := legalPagesTestHandler()

	for _, path := range []string{"/terms", "/refund-policy", "/privacy", "/contact"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()

		handler.Routes().ServeHTTP(rec, req)

		body := rec.Body.String()
		for _, want := range []string{
			`href="/"`,
			`href="/terms"`,
			`href="/refund-policy"`,
			`href="/privacy"`,
			`href="/contact"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: expected page to link to %q, body=%s", path, want, body)
			}
		}
	}
}

func TestTermsPageNamesPaddleAsMerchantOfRecord(t *testing.T) {
	handler := legalPagesTestHandler()

	req := httptest.NewRequest("GET", "/terms", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "merchant of record") {
		t.Fatalf("expected terms page to name Paddle as merchant of record, body=%s", body)
	}
	if !strings.Contains(body, "Paddle") {
		t.Fatalf("expected terms page to mention Paddle, body=%s", body)
	}
	if !strings.Contains(body, "1 EUR per month") {
		t.Fatalf("expected terms page to publicly state the 1 EUR/month price, body=%s", body)
	}
}

func TestRefundPolicyNamesPaddleAsMerchantOfRecordAndStatesWindow(t *testing.T) {
	handler := legalPagesTestHandler()

	req := httptest.NewRequest("GET", "/refund-policy", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "merchant of record") {
		t.Fatalf("expected refund policy to name Paddle as merchant of record, body=%s", body)
	}
	if !strings.Contains(body, "14 calendar days") {
		t.Fatalf("expected refund policy to state a concrete refund window, body=%s", body)
	}
}

func TestPrivacyPageCoversGDPRRightsAndPaddleAsIndependentController(t *testing.T) {
	handler := legalPagesTestHandler()

	req := httptest.NewRequest("GET", "/privacy", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		"Billing email",
		"Inbound email content",
		"IMAP/API access logs",
		"independent data controller",
		"erasure",
		"portable",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected privacy page to mention %q, body=%s", want, body)
		}
	}
}

func TestContactPageReferencesSupportEndpointAndNoInventedContactDetails(t *testing.T) {
	handler := legalPagesTestHandler()

	req := httptest.NewRequest("GET", "/contact", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "POST /v1/support/messages") {
		t.Fatalf("expected contact page to reference the support endpoint, body=%s", body)
	}
	if !strings.Contains(body, "hi@truevipaccess.com") {
		t.Fatalf("expected contact page to give the general contact email, body=%s", body)
	}
}

func TestHomePageLinksToLegalPages(t *testing.T) {
	handler := legalPagesTestHandler()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected home page to still return 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{`href="/terms"`, `href="/refund-policy"`, `href="/privacy"`, `href="/contact"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected home page footer to link to %q, body=%s", want, body)
		}
	}
}
