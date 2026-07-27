package httpapi

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

// paddleJSScriptSrc is Paddle's documented CDN origin for Paddle.js (Paddle
// Billing v2). It is also the sole script-src origin allowed by
// paddleContentSecurityPolicy below.
const paddleJSScriptSrc = "https://cdn.paddle.com/paddle/v2/paddle.js"

// paddleContentSecurityPolicy locks the checkout/success pages down to what
// is actually confirmed: script-src to 'self' plus Paddle's documented CDN
// origin (verified against Paddle's own docs). Inline <style> blocks (this
// codebase's existing template convention, see paymentSuccessHTMLTemplate)
// require 'unsafe-inline' on style-src; there is no nonce infrastructure
// here to tighten that further without a broader refactor.
//
// frame-src/connect-src are deliberately left unrestricted here (not
// tightened to Paddle's checkout origins) — see
// paddleContentSecurityPolicyReportOnly below for why.
const paddleContentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self' " + paddleJSScriptSrc + "; " +
	"style-src 'self' 'unsafe-inline'; " +
	"frame-src *; " +
	"connect-src *; " +
	"base-uri 'none'"

// paddleContentSecurityPolicyReportOnly carries the candidate frame-src/
// connect-src restriction (Paddle's checkout overlay iframe origins,
// sandbox and live) as report-only, not enforced. Unlike the CDN script
// origin, no authoritative source (Paddle's own docs, official starter
// kits, or a reputable third party) was found confirming these are the
// actual overlay-iframe origins as opposed to the hosted-checkout-redirect
// domain used in transaction.checkout.url — a different thing. Enforcing a
// wrong value here would silently break "Open payment window" the moment
// Paddle.Checkout.open() tries to load the overlay. Once the plan's
// mandated manual sandbox smoke test confirms the true origin (via
// devtools/report violations), flip this to enforced with the verified
// value and fold it back into paddleContentSecurityPolicy.
const paddleContentSecurityPolicyReportOnly = "frame-src https://checkout.paddle.com https://sandbox-checkout.paddle.com; " +
	"connect-src https://checkout.paddle.com https://sandbox-checkout.paddle.com"

func setPaddleCSPHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", paddleContentSecurityPolicy)
	w.Header().Set("Content-Security-Policy-Report-Only", paddleContentSecurityPolicyReportOnly)
}

// paddleTxnIDPattern matches Paddle Billing's transaction ID shape.
var paddleTxnIDPattern = regexp.MustCompile(`^txn_[a-z0-9]+$`)

// paddleCheckoutPriceLabel mirrors the public price already advertised on
// the home page (see homePageHTMLTemplate's lede). It is a display string,
// not derived from MAILBOX_PRICE_CENTS, which is a legacy Stripe-only
// config value known to disagree with the public price.
const paddleCheckoutPriceLabel = "1 EUR / month"

// handlePaddleCheckout is the page the payment-link email points customers
// at. It validates the Paddle transaction server-side before rendering
// anything, so it never shows a stale or already-resolved checkout overlay.
func (h *Handler) handlePaddleCheckout(w http.ResponseWriter, r *http.Request) {
	setPaddleCSPHeader(w)

	if h.paymentGateway == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("payment gateway not configured"))
		return
	}

	txnID := strings.TrimSpace(r.URL.Query().Get("_ptxn"))
	if !paddleTxnIDPattern.MatchString(txnID) {
		h.renderPaddleCheckoutInvalid(w)
		return
	}

	session, err := h.paymentGateway.GetPaymentSession(r.Context(), txnID)
	if err != nil {
		if errors.Is(err, ports.ErrPaymentSessionNotFound) {
			h.renderPaddleCheckoutInvalid(w)
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}

	switch session.Status {
	case ports.PaymentSessionStatusSucceeded, ports.PaymentSessionStatusConfirmed:
		http.Redirect(w, r, "/v1/payments/paddle/success?txn_id="+url.QueryEscape(txnID), http.StatusFound)
		return
	case ports.PaymentSessionStatusOpen:
		// draft/ready — proceed to render the checkout overlay below.
	case ports.PaymentSessionStatusFailed, ports.PaymentSessionStatusExpired:
		h.renderPaddleCheckoutInvalid(w)
		return
	default:
		h.renderPaddleCheckoutInvalid(w)
		return
	}

	mailbox, err := h.mailboxService.GetMailboxByPaymentSessionID(r.Context(), txnID)
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("paddle checkout: no mailbox found for open payment session txn_id=%s: %v", txnID, err)
		}
		h.renderPaddleCheckoutInvalid(w)
		return
	}

	h.renderPaddleCheckoutPage(w, txnID, mailbox)
}

type paddleCheckoutView struct {
	TxnID        string
	MailboxID    string
	MailboxEmail string
	PriceLabel   string
	ClientToken  string
	Environment  string // "sandbox" or "production" — Paddle.js's own vocabulary
	PaddleJSSrc  string
}

func (h *Handler) renderPaddleCheckoutPage(w http.ResponseWriter, txnID string, mailbox *domain.Mailbox) {
	email := mailbox.IMAPUsername
	if email == "" {
		email = mailbox.OwnerEmail
	}

	jsEnvironment := "production"
	if strings.EqualFold(h.paddleEnvironment, "sandbox") {
		jsEnvironment = "sandbox"
	}

	view := paddleCheckoutView{
		TxnID:        txnID,
		MailboxID:    mailbox.ID,
		MailboxEmail: email,
		PriceLabel:   paddleCheckoutPriceLabel,
		ClientToken:  h.paddleClientToken,
		Environment:  jsEnvironment,
		PaddleJSSrc:  paddleJSScriptSrc,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := paddleCheckoutTmpl.Execute(w, view); err != nil && h.logger != nil {
		h.logger.Printf("paddle checkout template render error: %v", err)
	}
}

func (h *Handler) renderPaddleCheckoutInvalid(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = paddleCheckoutInvalidTmpl.Execute(w, nil)
}

// handlePaddleSuccess is a UX-only confirmation page. It performs no
// mutating call — the webhook (U4) is the sole activation authority — and
// validates txn_id before any templating so a malformed value never reaches
// html/template.
func (h *Handler) handlePaddleSuccess(w http.ResponseWriter, r *http.Request) {
	setPaddleCSPHeader(w)

	txnID := strings.TrimSpace(r.URL.Query().Get("txn_id"))
	if !paddleTxnIDPattern.MatchString(txnID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid txn_id"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := paddleSuccessTmpl.Execute(w, paddleSuccessView{TxnID: txnID}); err != nil && h.logger != nil {
		h.logger.Printf("paddle success template render error: %v", err)
	}
}

type paddleSuccessView struct {
	TxnID string
}

var paddleCheckoutTmpl = template.Must(template.New("paddle_checkout").Parse(paddleCheckoutHTMLSource))

var paddleCheckoutInvalidTmpl = template.Must(template.New("paddle_checkout_invalid").Parse(paddleCheckoutInvalidHTMLSource))

var paddleSuccessTmpl = template.Must(template.New("paddle_success").Parse(paddleSuccessHTMLSource))

const paddleCheckoutInvalidHTMLSource = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Payment link no longer valid — TrueVIP Access</title>
  <style>
    body { font-family: Georgia, "Times New Roman", serif; background:#f4efe4; color:#17222d; margin:0; }
    main { max-width: 560px; margin: 0 auto; padding: 64px 20px; }
    .button { display:inline-block; margin-top:16px; padding:12px 20px; border-radius:999px; background:#a23b2a; color:#fffaf0; text-decoration:none; font-weight:700; }
  </style>
</head>
<body>
  <main>
    <h1>This payment link is no longer valid</h1>
    <p>It was canceled, expired, or does not exist. Start a new claim to get a fresh payment link.</p>
    <a class="button" href="/">Back to home</a>
  </main>
</body>
</html>`

const paddleCheckoutHTMLSource = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Complete your payment — TrueVIP Access</title>
  <style>
    body { font-family: Georgia, "Times New Roman", serif; background:#f4efe4; color:#17222d; margin:0; }
    main { max-width: 560px; margin: 0 auto; padding: 48px 20px; }
    h1 { font-size: 1.8rem; margin-bottom: 8px; }
    .card { border:1px solid #d8cdb7; border-radius: 14px; padding: 20px; background:#fffaf0; margin: 20px 0; }
    .row { display:flex; justify-content:space-between; padding:6px 0; border-bottom:1px solid #d8cdb7; }
    .row:last-child { border-bottom:none; }
    .button { display:inline-block; padding:12px 20px; border-radius:999px; background:#a23b2a; color:#fffaf0; border:none; font-weight:700; font-size:1rem; cursor:pointer; }
    .button:disabled { opacity:0.5; cursor:default; }
    #paddle-load-error, #paddle-retry, noscript p { display:none; border:1px solid #a23b2a; border-radius:14px; padding:16px; margin-top:16px; background:#fdeceb; }
    noscript p { display:block; }
  </style>
</head>
<body>
  <main>
    <h1>Complete your payment</h1>
    <p>Finish payment to activate this mailbox.</p>

    <div class="card">
      <div class="row"><span>Mailbox</span><span>{{.MailboxID}}</span></div>
      <div class="row"><span>Email</span><span>{{.MailboxEmail}}</span></div>
      <div class="row"><span>Price</span><span>{{.PriceLabel}}</span></div>
    </div>

    <noscript>
      <p>JavaScript is required to complete payment. Please enable JavaScript, or contact support at hi@truevipaccess.com.</p>
    </noscript>

    <button id="paddle-open-btn" class="button" onclick="openPaddleCheckout()" disabled>Open payment window</button>

    <div id="paddle-load-error">
      <strong>The payment window could not load.</strong> Please contact support at hi@truevipaccess.com.
    </div>
    <div id="paddle-retry">
      <strong>Payment was not completed.</strong> You can try again above.
    </div>

    <script src="{{.PaddleJSSrc}}" onerror="document.getElementById('paddle-load-error').style.display='block'"></script>
    <script>
      (function () {
        var txnId = "{{.TxnID}}";

        function showLoadError() {
          document.getElementById('paddle-load-error').style.display = 'block';
        }
        function showRetry() {
          document.getElementById('paddle-retry').style.display = 'block';
        }

        if (typeof Paddle === 'undefined') {
          showLoadError();
          return;
        }

        Paddle.Environment.set("{{.Environment}}");
        Paddle.Initialize({
          token: "{{.ClientToken}}",
          eventCallback: function (event) {
            // Exposed for the ops/paddle-checkout-confirm.js smoke-test
            // helper, which cannot rely on Paddle's third-party overlay DOM
            // and instead waits on Paddle.js's own documented event names
            // (checkout.loaded / checkout.completed / checkout.error /
            // checkout.closed). Harmless no-op for real users.
            window.__paddleEvents = window.__paddleEvents || [];
            if (event && event.name) {
              window.__paddleEvents.push(event.name);
            }
            if (event && (event.name === 'checkout.error' || event.name === 'checkout.closed')) {
              showRetry();
            }
          }
        });

        var openBtn = document.getElementById('paddle-open-btn');
        openBtn.disabled = false;
        window.openPaddleCheckout = function () {
          document.getElementById('paddle-retry').style.display = 'none';
          Paddle.Checkout.open({ transactionId: txnId });
        };
      })();
    </script>
  </main>
</body>
</html>`

const paddleSuccessHTMLSource = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Payment received — TrueVIP Access</title>
  <style>
    body { font-family: Georgia, "Times New Roman", serif; background:#f4efe4; color:#17222d; margin:0; }
    main { max-width: 560px; margin: 0 auto; padding: 64px 20px; text-align:center; }
    .button { display:inline-block; margin-top:16px; padding:12px 20px; border-radius:999px; background:#a23b2a; color:#fffaf0; text-decoration:none; font-weight:700; }
    .ref { font-size:0.85rem; color:#566575; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  </style>
</head>
<body>
  <main>
    <h1>Payment received</h1>
    <p>We're activating your mailbox and will email your credentials shortly.</p>
    <p class="ref">Reference: {{.TxnID}}</p>
    <a class="button" href="/">Back to home</a>
  </main>
</body>
</html>`
