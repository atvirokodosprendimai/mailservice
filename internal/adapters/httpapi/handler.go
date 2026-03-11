package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	"github.com/atvirokodosprendimai/mailservice/internal/adapters/identity/edproof"
	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type Config struct {
	AdminAPIKey         string
	StripeWebhookSecret string
	PolarWebhookSecret  string
	MaxConcurrentReqs   int
	BuildNumber         string
	CacheBuster         string
	KeyProofVerifier    ports.KeyProofVerifier
	PaymentGateway      ports.PaymentGateway
	MailboxService      *service.MailboxService
	AccountService      *service.AccountService
	Logger              *log.Logger
	Now                 func() time.Time
	EdproofHMACSecret   []byte
}

type Handler struct {
	adminAPIKey         string
	stripeWebhookSecret string
	polarWebhookSecret  string
	concurrencySem      chan struct{}
	keyProofVerifier    ports.KeyProofVerifier
	paymentGateway      ports.PaymentGateway
	mailboxService      *service.MailboxService
	accountService      *service.AccountService
	logger              *log.Logger
	now                 func() time.Time
	buildNumber         string
	cacheBuster         string
	edproofHMACSecret   []byte
}

type accountContextKey struct{}

func NewHandler(cfg Config) *Handler {
	var sem chan struct{}
	if cfg.MaxConcurrentReqs > 0 {
		sem = make(chan struct{}, cfg.MaxConcurrentReqs)
	}

	return &Handler{
		adminAPIKey:         cfg.AdminAPIKey,
		stripeWebhookSecret: cfg.StripeWebhookSecret,
		polarWebhookSecret:  cfg.PolarWebhookSecret,
		concurrencySem:      sem,
		keyProofVerifier:    cfg.KeyProofVerifier,
		paymentGateway:      cfg.PaymentGateway,
		mailboxService:      cfg.MailboxService,
		accountService:      cfg.AccountService,
		logger:              cfg.Logger,
		now:                 coalesceNow(cfg.Now),
		buildNumber:         fallbackString(cfg.BuildNumber, "dev"),
		cacheBuster:         fallbackString(cfg.CacheBuster, fallbackString(cfg.BuildNumber, "dev")),
		edproofHMACSecret:   cfg.EdproofHMACSecret,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.handleHome)
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("POST /v1/accounts", h.handleCreateAccount)
	mux.HandleFunc("POST /v1/auth/refresh", h.handleRefreshAuth)
	mux.HandleFunc("POST /v1/accounts/recovery/start", h.handleStartRecovery)
	mux.HandleFunc("POST /v1/accounts/recovery/complete", h.handleCompleteRecovery)
	mux.HandleFunc("GET /v1/accounts/recovery/complete", h.handleCompleteRecoveryByLink)
	mux.HandleFunc("GET /v1/mailboxes", h.withAccountToken(h.handleListMailboxes))
	mux.HandleFunc("POST /v1/mailboxes", h.withAccountToken(h.handleCreateMailbox))
	mux.HandleFunc("POST /v1/auth/challenge", h.handleAuthChallenge)
	mux.HandleFunc("POST /v1/mailboxes/claim", h.handleClaimMailbox)
	mux.HandleFunc("GET /v1/mailboxes/{id}", h.withAccountToken(h.handleGetMailbox))
	mux.HandleFunc("POST /v1/access/resolve", h.handleResolveAccess)
	mux.HandleFunc("GET /v1/payments/polar/success", h.handlePolarSuccess)
	mux.HandleFunc("POST /v1/webhooks/polar", h.handlePolarWebhook)
	mux.HandleFunc("POST /v1/imap/resolve", h.handleResolveIMAP)
	mux.HandleFunc("POST /v1/imap/messages", h.handleListIMAPMessages)
	mux.HandleFunc("POST /v1/imap/messages/get", h.handleGetIMAPMessageByUID)
	mux.HandleFunc("POST /v1/webhooks/stripe", h.handleStripeWebhook)
	mux.HandleFunc("POST /admin/mailboxes/reprovision", h.withAdminKey(h.handleReprovisionMailbox))
	mux.HandleFunc("GET /mock/pay/{sessionID}", h.handleMockPay)
	handler := http.Handler(mux)
	if h.concurrencySem != nil {
		handler = h.withGlobalSemaphore(handler)
	}
	return handler
}

func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, renderHomePageHTML(h.buildNumber, h.cacheBuster))
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func renderHomePageHTML(buildNumber string, cacheBuster string) string {
	return fmt.Sprintf(homePageHTMLTemplate,
		html.EscapeString(buildNumber),
		html.EscapeString(cacheBuster),
		html.EscapeString(cacheBuster),
		homePageAgentPrompt,
	)
}

var homePageHTMLTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Cache-Control" content="no-store, max-age=0">
  <meta http-equiv="Pragma" content="no-cache">
  <meta http-equiv="Expires" content="0">
  <title>TrueVIP Access Mailbox</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f4efe4;
      --ink: #17222d;
      --muted: #566575;
      --card: #fffaf0;
      --line: #d8cdb7;
      --accent: #a23b2a;
      --accent-ink: #fffaf0;
      --code: #f0e7d5;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      background:
        radial-gradient(circle at top left, rgba(162,59,42,0.12), transparent 28%%),
        linear-gradient(180deg, #f7f2e8 0%%, var(--bg) 100%%);
      color: var(--ink);
    }
    main {
      max-width: 880px;
      margin: 0 auto;
      padding: 48px 20px 72px;
    }
    .eyebrow {
      display: inline-block;
      margin-bottom: 14px;
      padding: 6px 10px;
      border: 1px solid var(--line);
      border-radius: 999px;
      font: 600 12px/1.2 ui-monospace, SFMono-Regular, Menlo, monospace;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted);
      background: rgba(255, 250, 240, 0.75);
    }
    h1 {
      margin: 0 0 12px;
      font-size: clamp(2.5rem, 5vw, 4.6rem);
      line-height: 0.95;
      letter-spacing: -0.04em;
    }
    .lede {
      max-width: 42rem;
      margin: 0 0 24px;
      font-size: 1.15rem;
      line-height: 1.6;
      color: var(--muted);
    }
    .meta {
      margin: 0 0 20px;
      color: var(--muted);
      font: 500 0.9rem/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
    }
    .rules {
      display: grid;
      gap: 12px;
      margin: 24px 0 32px;
    }
    .rule, .card {
      padding: 18px 20px;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: rgba(255, 250, 240, 0.9);
    }
    .rule strong { display: block; margin-bottom: 6px; }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      margin: 0 0 40px;
    }
    .button {
      display: inline-block;
      padding: 12px 16px;
      border-radius: 999px;
      text-decoration: none;
      font-weight: 700;
      border: 1px solid var(--ink);
    }
    .button.primary {
      background: var(--accent);
      border-color: var(--accent);
      color: var(--accent-ink);
    }
    .button.secondary {
      color: var(--ink);
      background: transparent;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 16px;
      margin: 0 0 28px;
    }
    h2 {
      margin: 0 0 14px;
      font-size: 1.35rem;
    }
    p, li { line-height: 1.6; }
    ul, ol { margin: 0; padding-left: 20px; }
    code {
      padding: 0.1em 0.35em;
      border-radius: 6px;
      background: var(--code);
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.95em;
    }
    .instruction {
      margin-top: 24px;
      padding: 22px;
      border-radius: 22px;
      border: 1px solid var(--line);
      background: var(--card);
    }
    .prompt {
      margin: 0 0 18px;
      padding: 16px 18px;
      border-radius: 14px;
      background: #181b20;
      color: #7ef7d3;
      border: 1px solid #2d3540;
      font: 500 0.98rem/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;
      white-space: pre-wrap;
    }
  </style>
</head>
<body>
  <main>
    <div class="eyebrow">Inbound Mailboxes For Agents</div>
    <h1>Stable mailbox identity, bound to a key.</h1>
    <p class="lede">
      Same key, same mailbox. Different key, different mailbox. Pay monthly. Read inbound mail over IMAP.
      Pricing: 1 EUR/month per mailbox (100 MB storage). No SMTP. No outbound sending.
    </p>
    <p class="meta">Build: <code>%s</code> | Cache buster: <code>%s</code></p>

    <div class="rules">
      <div class="rule">
        <strong>The key is the mailbox identity.</strong>
        EdProof is the key proof used to identify the mailbox. It is not a login account, not a billing email,
        and not a payment method.
      </div>
      <div class="rule">
        <strong>The billing email is only for billing.</strong>
        The billing email is only where the invoice goes.
      </div>
      <div class="rule">
        <strong>This product is inbound-only.</strong>
        You get inbound delivery and IMAP read access. You do not get SMTP submission or relay access.
      </div>
    </div>

    <div class="actions">
      <a class="button primary" href="#start">How to start</a>
      <a class="button secondary" href="/healthz?cb=%s">Health</a>
    </div>

    <div class="grid">
      <section class="card">
        <h2>Bring a key</h2>
        <p>If your agent already has an Ed25519 key, use it. If it does not, generate one with <code>ssh-keygen -t ed25519 -f identity -C "entity@context"</code>.</p>
      </section>
      <section class="card">
        <h2>Claim a mailbox</h2>
        <p>Call <code>POST /v1/mailboxes/claim</code> with <code>billing_email</code> and <code>edproof</code>.</p>
      </section>
      <section class="card">
        <h2>Pay and read mail</h2>
        <p>Pay from the emailed link, then call <code>POST /v1/access/resolve</code> with the same key proof.</p>
      </section>
    </div>

    <section id="start" class="instruction">
      <h2>Give this to your agent</h2>
      <div class="prompt">%s</div>
      <ol>
        <li>If no EdProof key exists locally, generate one and continue.</li>
        <li>If you want the same mailbox later, keep the same EdProof key. A different key gets a different mailbox.</li>
        <li>Call <code>POST /v1/mailboxes/claim</code> with <code>billing_email</code> and <code>edproof</code>.</li>
        <li>Pay from the emailed payment link.</li>
        <li>Call <code>POST /v1/access/resolve</code> with <code>protocol=imap</code> and <code>edproof</code>.</li>
        <li>Read mail using the returned IMAP details.</li>
      </ol>
    </section>

    <section class="instruction" style="margin-top:24px">
      <h2>Reading mail</h2>
      <p style="color:var(--muted);margin:0 0 18px">Two options: HTTP API (easiest for agents) or direct IMAP (for mail clients and raw protocol access).</p>

      <h3>Option A: HTTP API (recommended for agents)</h3>
      <p>After payment, resolve access credentials, then use the HTTP endpoints. No IMAP library required.</p>

      <div class="prompt">## Step 1 — resolve credentials
curl -X POST https://truevipaccess.com/v1/access/resolve \
  -H 'Content-Type: application/json' \
  -d '{"protocol":"imap","edproof":"&lt;your-edproof&gt;"}'

# Response:
# {"mailbox_id":"...","host":"mail.truevipaccess.com","port":143,
#  "username":"...@truevipaccess.com","password":"...","email":"..."}

## Step 2 — list messages
curl -X POST https://truevipaccess.com/v1/imap/messages \
  -H 'Authorization: Bearer &lt;api_token&gt;' \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"&lt;mailbox_access_token&gt;","unread_only":true,"include_body":true}'

## Step 3 — get a single message by UID
curl -X POST https://truevipaccess.com/v1/imap/messages/get \
  -H 'Authorization: Bearer &lt;api_token&gt;' \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"&lt;mailbox_access_token&gt;","uid":1,"include_body":true}'</div>

      <p style="font-size:0.95rem;color:var(--muted)"><strong>Agent note:</strong> <code>access_token</code> comes from the mailbox claim/create response (returned when mailbox is usable). <code>api_token</code> is the account-level auth token used in the Authorization header.</p>

      <h3>Option B: Direct IMAP</h3>
      <p>Connect with any IMAP client using the credentials from <code>/v1/access/resolve</code>.</p>

      <div class="prompt">## Python (TLS on port 993)
import imaplib
imap = imaplib.IMAP4_SSL("mail.truevipaccess.com", 993)
imap.login(username, password)
imap.select("INBOX", readonly=True)
_, data = imap.search(None, "UNSEEN")
for num in data[0].split():
    _, msg = imap.fetch(num, "(RFC822)")
    print(msg[0][1])
imap.logout()

## curl (test connectivity)
curl -v --url "imaps://mail.truevipaccess.com:993/INBOX" \
  --user "user@truevipaccess.com:password"</div>

      <div style="margin-top:18px">
        <strong>Mail client settings (Thunderbird, Apple Mail, etc.)</strong>
        <ul style="margin-top:8px">
          <li><strong>Protocol:</strong> IMAP</li>
          <li><strong>Host:</strong> <code>mail.truevipaccess.com</code></li>
          <li><strong>Port:</strong> <code>993</code> (TLS) or <code>143</code> (STARTTLS)</li>
          <li><strong>Encryption:</strong> SSL/TLS (recommended) or STARTTLS</li>
          <li><strong>Authentication:</strong> Normal password</li>
          <li><strong>Username/Password:</strong> from the <code>/v1/access/resolve</code> response</li>
        </ul>
      </div>
    </section>
  </main>
</body>
</html>
`

const homePageAgentPrompt = `If no EdProof key exists locally, generate an Ed25519 key pair with: ssh-keygen -t ed25519 -f identity -C "entity@context"
Use identity.pub as the public key and keep identity private.
The SHA-256 fingerprint from ssh-keygen -l -E sha256 -f identity.pub is the stable EdProof identifier.
EdProof is the key proof used to identify the mailbox.
If the same mailbox is required, reuse the same EdProof key.
Do not ask the operator unless key generation is impossible or the same mailbox is required but the existing key is unavailable.`

type createAccountRequest struct {
	OwnerEmail string `json:"owner_email"`
}

type accountView struct {
	ID                    string  `json:"id"`
	OwnerEmail            string  `json:"owner_email"`
	APIToken              string  `json:"api_token"`
	RefreshToken          string  `json:"refresh_token,omitempty"`
	SubscriptionExpiresAt *string `json:"subscription_expires_at,omitempty"`
}

func (h *Handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	account, tokens, err := h.accountService.CreateAccount(r.Context(), req.OwnerEmail)
	if err != nil {
		if errors.Is(err, ports.ErrAccountExists) {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "email_sent_if_exists"})
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	view := accountView{
		ID:           account.ID,
		OwnerEmail:   account.OwnerEmail,
		APIToken:     tokens.APIToken,
		RefreshToken: tokens.RefreshToken,
	}
	if account.SubscriptionExpiresAt != nil {
		v := account.SubscriptionExpiresAt.Format(time.RFC3339)
		view.SubscriptionExpiresAt = &v
	}
	writeJSON(w, http.StatusCreated, view)
}

type refreshAuthRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) handleRefreshAuth(w http.ResponseWriter, r *http.Request) {
	var req refreshAuthRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	account, tokens, err := h.accountService.RefreshAccess(r.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrRefreshNotFound):
			writeError(w, http.StatusUnauthorized, err)
		case errors.Is(err, ports.ErrRefreshExpired):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	view := accountView{
		ID:           account.ID,
		OwnerEmail:   account.OwnerEmail,
		APIToken:     tokens.APIToken,
		RefreshToken: tokens.RefreshToken,
	}
	if account.SubscriptionExpiresAt != nil {
		v := account.SubscriptionExpiresAt.Format(time.RFC3339)
		view.SubscriptionExpiresAt = &v
	}
	writeJSON(w, http.StatusOK, view)
}

type accountRecoveryRequest struct {
	OwnerEmail string `json:"owner_email"`
}

func (h *Handler) handleStartRecovery(w http.ResponseWriter, r *http.Request) {
	var req accountRecoveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.accountService.StartRecovery(r.Context(), req.OwnerEmail); err != nil {
		if errors.Is(err, ports.ErrRateLimitReached) {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "email_sent_if_exists"})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "email_sent_if_exists"})
}

type completeRecoveryRequest struct {
	Token string `json:"token"`
}

func (h *Handler) handleCompleteRecovery(w http.ResponseWriter, r *http.Request) {
	var req completeRecoveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	account, tokens, err := h.accountService.CompleteRecoveryByToken(r.Context(), req.Token)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrRecoveryInvalid):
			writeError(w, http.StatusUnauthorized, err)
		case errors.Is(err, ports.ErrRecoveryExpired):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	view := accountView{
		ID:           account.ID,
		OwnerEmail:   account.OwnerEmail,
		APIToken:     tokens.APIToken,
		RefreshToken: tokens.RefreshToken,
	}
	if account.SubscriptionExpiresAt != nil {
		v := account.SubscriptionExpiresAt.Format(time.RFC3339)
		view.SubscriptionExpiresAt = &v
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) handleCompleteRecoveryByLink(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing token"))
		return
	}

	account, tokens, err := h.accountService.CompleteRecoveryByToken(r.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrRecoveryInvalid):
			writeError(w, http.StatusUnauthorized, err)
		case errors.Is(err, ports.ErrRecoveryExpired):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(
		"Recovery completed. Save these credentials securely:\n" +
			"account_id=" + account.ID + "\n" +
			"owner_email=" + account.OwnerEmail + "\n" +
			"api_token=" + tokens.APIToken + "\n" +
			"refresh_token=" + tokens.RefreshToken + "\n",
	))
}

func (h *Handler) handleCreateMailbox(w http.ResponseWriter, r *http.Request) {
	account, err := accountFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	mailbox, created, err := h.mailboxService.CreateMailbox(r.Context(), service.CreateMailboxRequest{Account: account})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	writeJSON(w, status, mailboxResponse(mailbox))
}

type authChallengeRequest struct {
	PublicKey string `json:"public_key"`
}

func (h *Handler) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if len(h.edproofHMACSecret) == 0 {
		writeError(w, http.StatusServiceUnavailable, errors.New("challenge-response not configured"))
		return
	}

	var req authChallengeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	challenge, err := edproof.GenerateChallenge(req.PublicKey, h.edproofHMACSecret, h.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"challenge":  challenge,
		"expires_in": 30,
	})
}

type claimMailboxRequest struct {
	BillingEmail string `json:"billing_email"`
	EDProof      string `json:"edproof"`
	Challenge    string `json:"challenge"`
	Signature    string `json:"signature"`
}

func (h *Handler) handleClaimMailbox(w http.ResponseWriter, r *http.Request) {
	var req claimMailboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if h.keyProofVerifier == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("key proof verifier not configured"))
		return
	}

	key, err := h.verifyEdproof(r.Context(), req.EDProof, req.Challenge, req.Signature)
	if err != nil {
		writeEdproofError(w, err)
		return
	}

	mailbox, created, err := h.mailboxService.ClaimMailbox(r.Context(), req.BillingEmail, *key)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrInvalidKeyProof):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, mailboxResponse(mailbox))
}

func (h *Handler) handleListMailboxes(w http.ResponseWriter, r *http.Request) {
	account, err := accountFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	mailboxes, err := h.mailboxService.ListMailboxesForAccount(r.Context(), account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	items := make([]mailboxView, 0, len(mailboxes))
	for i := range mailboxes {
		m := mailboxes[i]
		items = append(items, mailboxResponse(&m))
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleGetMailbox(w http.ResponseWriter, r *http.Request) {
	account, err := accountFromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	id := r.PathValue("id")
	mailbox, err := h.mailboxService.GetMailboxForAccount(r.Context(), id, account.ID)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrMailboxNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrForbidden):
			writeError(w, http.StatusForbidden, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, mailboxResponse(mailbox))
}

type resolveIMAPRequest struct {
	AccessToken string `json:"access_token"`
}

type resolveAccessRequest struct {
	Protocol  string `json:"protocol"`
	EDProof   string `json:"edproof"`
	Challenge string `json:"challenge"`
	Signature string `json:"signature"`
}

func (h *Handler) handleResolveIMAP(w http.ResponseWriter, r *http.Request) {
	var req resolveIMAPRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := h.mailboxService.ResolveAccessByToken(r.Context(), req.AccessToken, "imap")
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrMailboxNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrMailboxNotUsable):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "waiting_payment"})
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, resolveAccessResponse(result))
}

func (h *Handler) handleResolveAccess(w http.ResponseWriter, r *http.Request) {
	var req resolveAccessRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(strings.ToLower(req.Protocol)) != "imap" {
		writeError(w, http.StatusBadRequest, errors.New("unsupported protocol"))
		return
	}
	if h.keyProofVerifier == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("key proof verifier not configured"))
		return
	}

	key, err := h.verifyEdproof(r.Context(), req.EDProof, req.Challenge, req.Signature)
	if err != nil {
		writeEdproofError(w, err)
		return
	}

	result, err := h.mailboxService.ResolveAccessByKey(r.Context(), *key, req.Protocol)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrMailboxNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrMailboxNotUsable):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "waiting_payment"})
		case errors.Is(err, ports.ErrInvalidKeyProof):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, resolveAccessResponse(result))
}

// verifyEdproof handles challenge-response verification when HMAC secret is configured,
// or falls back to passthrough verification when not configured.
func (h *Handler) verifyEdproof(ctx context.Context, pubkey, challenge, signature string) (*ports.VerifiedKey, error) {
	if len(h.edproofHMACSecret) > 0 {
		// Challenge-response mode: challenge and signature are required
		if challenge == "" || signature == "" {
			return nil, errChallengeRequired
		}
		if err := edproof.VerifyChallenge(challenge, pubkey, h.edproofHMACSecret, 30*time.Second, h.now()); err != nil {
			return nil, err
		}
		if err := edproof.VerifySignature(challenge, pubkey, signature); err != nil {
			return nil, err
		}
	}
	// Verify pubkey format and extract fingerprint (always needed)
	return h.keyProofVerifier.Verify(ctx, pubkey)
}

var errChallengeRequired = errors.New("edproof now requires challenge-response — call POST /v1/auth/challenge first")

func writeEdproofError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errChallengeRequired):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, edproof.ErrChallengeExpired):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, edproof.ErrChallengeTampered):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, edproof.ErrChallengeFuture):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, edproof.ErrSignatureInvalid):
		writeError(w, http.StatusUnauthorized, err)
	case errors.Is(err, ports.ErrInvalidKeyProof):
		writeError(w, http.StatusUnauthorized, err)
	default:
		writeError(w, http.StatusServiceUnavailable, err)
	}
}

type listMessagesRequest struct {
	AccessToken string `json:"access_token"`
	Limit       int    `json:"limit,omitempty"`
	UnreadOnly  *bool  `json:"unread_only,omitempty"`
	IncludeBody *bool  `json:"include_body,omitempty"`
}

func (h *Handler) handleListIMAPMessages(w http.ResponseWriter, r *http.Request) {
	var req listMessagesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	unreadOnly := true
	if req.UnreadOnly != nil {
		unreadOnly = *req.UnreadOnly
	}

	includeBody := false
	if req.IncludeBody != nil {
		includeBody = *req.IncludeBody
	}

	messages, err := h.mailboxService.ListMessagesByToken(r.Context(), req.AccessToken, req.Limit, unreadOnly, includeBody)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrMailboxNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrMailboxNotUsable):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "waiting_payment"})
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"provider": "imap",
		"messages": messages,
	})
}

type getMessageByUIDRequest struct {
	AccessToken string `json:"access_token"`
	UID         uint32 `json:"uid"`
	IncludeBody *bool  `json:"include_body,omitempty"`
}

func (h *Handler) handleGetIMAPMessageByUID(w http.ResponseWriter, r *http.Request) {
	var req getMessageByUIDRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.UID == 0 {
		writeError(w, http.StatusBadRequest, errors.New("uid must be > 0"))
		return
	}

	includeBody := true
	if req.IncludeBody != nil {
		includeBody = *req.IncludeBody
	}

	message, err := h.mailboxService.GetMessageByUIDToken(r.Context(), req.AccessToken, req.UID, includeBody)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrMailboxNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrMessageNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, ports.ErrMailboxNotUsable):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "waiting_payment"})
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"provider": "imap",
		"message":  message,
	})
}

func (h *Handler) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if h.stripeWebhookSecret == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("stripe webhook secret not configured"))
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.stripeWebhookSecret)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	sessionEventTypes := map[stripe.EventType]bool{
		stripe.EventTypeCheckoutSessionCompleted:             true,
		stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded: true,
	}

	if !sessionEventTypes[event.Type] {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	var checkoutSession stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &checkoutSession); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if checkoutSession.ID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing checkout session id"))
		return
	}

	if _, err := h.mailboxService.MarkMailboxPaid(r.Context(), checkoutSession.ID); err != nil {
		if errors.Is(err, ports.ErrMailboxNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleMockPay(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if !strings.HasPrefix(sessionID, "mock_") {
		writeError(w, http.StatusBadRequest, errors.New("not a mock session"))
		return
	}

	if _, err := h.mailboxService.MarkMailboxPaid(context.Background(), sessionID); err != nil {
		if errors.Is(err, ports.ErrMailboxNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "paid"})
}

func (h *Handler) handlePolarSuccess(w http.ResponseWriter, r *http.Request) {
	if h.paymentGateway == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("payment gateway not configured"))
		return
	}

	checkoutID := strings.TrimSpace(r.URL.Query().Get("checkout_id"))
	if checkoutID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing checkout_id"))
		return
	}

	session, err := h.paymentGateway.GetPaymentSession(r.Context(), checkoutID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	if !isSuccessfulPayment(session.Status) {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "waiting_payment"})
		return
	}

	mailbox, err := h.mailboxService.MarkMailboxPaid(r.Context(), session.SessionID)
	if err != nil {
		if errors.Is(err, ports.ErrMailboxNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, polarSuccessView{
		Status:    "ok",
		MailboxID: mailbox.ID,
	})
}

func (h *Handler) handlePolarWebhook(w http.ResponseWriter, r *http.Request) {
	if h.paymentGateway == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("payment gateway not configured"))
		return
	}
	if strings.TrimSpace(h.polarWebhookSecret) == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("polar webhook secret not configured"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	headers := map[string]string{
		"webhook-id":        r.Header.Get("webhook-id"),
		"webhook-timestamp": r.Header.Get("webhook-timestamp"),
		"webhook-signature": r.Header.Get("webhook-signature"),
	}
	if err := verifyPolarWebhook(h.polarWebhookSecret, headers, body, h.now()); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	event, err := parsePolarWebhook(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	checkoutID := polarCheckoutID(event)
	if checkoutID == "" {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
		return
	}

	session, err := h.paymentGateway.GetPaymentSession(r.Context(), checkoutID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !isSuccessfulPayment(session.Status) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
		return
	}

	if _, err := h.mailboxService.MarkMailboxPaid(r.Context(), session.SessionID); err != nil {
		if errors.Is(err, ports.ErrMailboxNotFound) {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
}

func (h *Handler) withAccountToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-API-Token")
		if token == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}

		account, err := h.accountService.GetByToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("invalid api token"))
			return
		}

		ctx := context.WithValue(r.Context(), accountContextKey{}, account)
		next(w, r.WithContext(ctx))
	}
}

func accountFromContext(ctx context.Context) (*domain.Account, error) {
	v := ctx.Value(accountContextKey{})
	if v == nil {
		return nil, errors.New("account not found in context")
	}
	account, ok := v.(*domain.Account)
	if !ok {
		return nil, errors.New("invalid account context")
	}
	return account, nil
}

type mailboxView struct {
	ID          string               `json:"id"`
	Status      domain.MailboxStatus `json:"status"`
	Usable      bool                 `json:"usable"`
	PaymentURL  string               `json:"payment_url"`
	ExpiresAt   *string              `json:"expires_at,omitempty"`
	AccessToken string               `json:"access_token,omitempty"`
}

type resolveAccessView struct {
	MailboxID   string `json:"mailbox_id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	AccessToken string `json:"access_token,omitempty"`
}

type polarSuccessView struct {
	Status    string `json:"status"`
	MailboxID string `json:"mailbox_id"`
}

func mailboxResponse(mailbox *domain.Mailbox) mailboxView {
	resp := mailboxView{
		ID:         mailbox.ID,
		Status:     mailbox.Status,
		Usable:     mailbox.Usable(),
		PaymentURL: mailbox.PaymentURL,
	}
	if mailbox.ExpiresAt != nil {
		expires := mailbox.ExpiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &expires
	}
	if mailbox.Usable() {
		resp.AccessToken = mailbox.AccessToken
	}
	return resp
}

func resolveAccessResponse(result *service.ResolveAccessResult) resolveAccessView {
	return resolveAccessView{
		MailboxID:   result.MailboxID,
		Host:        result.Host,
		Port:        result.Port,
		Username:    result.Username,
		Password:    result.Password,
		Email:       result.Email,
		AccessToken: result.AccessToken,
	}
}

func decodeJSON(r *http.Request, into any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("request body must contain only one JSON object")
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request body has trailing content")
	}
	return nil
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func isSuccessfulPayment(status ports.PaymentSessionStatus) bool {
	return status == ports.PaymentSessionStatusSucceeded || status == ports.PaymentSessionStatusConfirmed
}

func coalesceNow(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return func() time.Time {
		return time.Now().UTC()
	}
}

func (h *Handler) withAdminKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.adminAPIKey == "" {
			writeError(w, http.StatusServiceUnavailable, errors.New("admin API not configured"))
			return
		}
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if strings.TrimSpace(token) == "" || token != h.adminAPIKey {
			writeError(w, http.StatusUnauthorized, errors.New("invalid admin key"))
			return
		}
		next(w, r)
	}
}

func (h *Handler) handleReprovisionMailbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MailboxID  string `json:"mailbox_id"`
		OwnerEmail string `json:"owner_email"`
		PublicKey  string `json:"public_key"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.MailboxID == "" || req.OwnerEmail == "" || req.PublicKey == "" || req.ExpiresAt == "" {
		writeError(w, http.StatusBadRequest, errors.New("mailbox_id, owner_email, public_key, and expires_at are required"))
		return
	}
	fingerprint, err := edproof.FingerprintFromPubkey(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid public_key: %w", err))
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid expires_at (use RFC3339): %w", err))
		return
	}

	mailbox, err := h.mailboxService.ReprovisionMailbox(r.Context(), service.ReprovisionRequest{
		MailboxID:      req.MailboxID,
		OwnerEmail:     req.OwnerEmail,
		KeyFingerprint: fingerprint,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         mailbox.ID,
		"email":      mailbox.IMAPUsername + "@" + h.mailboxService.MailDomain(),
		"status":     mailbox.Status,
		"expires_at": mailbox.ExpiresAt,
	})
}

func fallbackString(value string, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}
