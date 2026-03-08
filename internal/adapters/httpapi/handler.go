package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type Config struct {
	StripeWebhookSecret string
	MaxConcurrentReqs   int
	KeyProofVerifier    ports.KeyProofVerifier
	MailboxService      *service.MailboxService
	AccountService      *service.AccountService
	Logger              *log.Logger
}

type Handler struct {
	stripeWebhookSecret string
	concurrencySem      chan struct{}
	keyProofVerifier    ports.KeyProofVerifier
	mailboxService      *service.MailboxService
	accountService      *service.AccountService
	logger              *log.Logger
}

type accountContextKey struct{}

func NewHandler(cfg Config) *Handler {
	var sem chan struct{}
	if cfg.MaxConcurrentReqs > 0 {
		sem = make(chan struct{}, cfg.MaxConcurrentReqs)
	}

	return &Handler{
		stripeWebhookSecret: cfg.StripeWebhookSecret,
		concurrencySem:      sem,
		keyProofVerifier:    cfg.KeyProofVerifier,
		mailboxService:      cfg.MailboxService,
		accountService:      cfg.AccountService,
		logger:              cfg.Logger,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("POST /v1/accounts", h.handleCreateAccount)
	mux.HandleFunc("POST /v1/auth/refresh", h.handleRefreshAuth)
	mux.HandleFunc("POST /v1/accounts/recovery/start", h.handleStartRecovery)
	mux.HandleFunc("POST /v1/accounts/recovery/complete", h.handleCompleteRecovery)
	mux.HandleFunc("GET /v1/accounts/recovery/complete", h.handleCompleteRecoveryByLink)
	mux.HandleFunc("GET /v1/mailboxes", h.withAccountToken(h.handleListMailboxes))
	mux.HandleFunc("POST /v1/mailboxes", h.withAccountToken(h.handleCreateMailbox))
	mux.HandleFunc("POST /v1/mailboxes/claim", h.handleClaimMailbox)
	mux.HandleFunc("GET /v1/mailboxes/{id}", h.withAccountToken(h.handleGetMailbox))
	mux.HandleFunc("POST /v1/access/resolve", h.handleResolveAccess)
	mux.HandleFunc("POST /v1/imap/resolve", h.withAccountToken(h.handleResolveIMAP))
	mux.HandleFunc("POST /v1/imap/messages", h.withAccountToken(h.handleListIMAPMessages))
	mux.HandleFunc("POST /v1/imap/messages/get", h.withAccountToken(h.handleGetIMAPMessageByUID))
	mux.HandleFunc("POST /v1/webhooks/stripe", h.handleStripeWebhook)
	mux.HandleFunc("GET /mock/pay/{sessionID}", h.handleMockPay)
	handler := http.Handler(mux)
	if h.concurrencySem != nil {
		handler = h.withGlobalSemaphore(handler)
	}
	return handler
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

type claimMailboxRequest struct {
	BillingEmail string `json:"billing_email"`
	EDProof      string `json:"edproof"`
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

	key, err := h.keyProofVerifier.Verify(r.Context(), req.EDProof)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrInvalidKeyProof):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusServiceUnavailable, err)
		}
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
	Protocol string `json:"protocol"`
	EDProof  string `json:"edproof"`
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

	writeJSON(w, http.StatusOK, result)
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

	key, err := h.keyProofVerifier.Verify(r.Context(), req.EDProof)
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrInvalidKeyProof):
			writeError(w, http.StatusUnauthorized, err)
		default:
			writeError(w, http.StatusServiceUnavailable, err)
		}
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

	writeJSON(w, http.StatusOK, result)
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
