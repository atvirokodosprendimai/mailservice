package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/core/service"
	"github.com/atvirokodosprendimai/mailservice/internal/domain"
)

type Config struct {
	StripeWebhookSecret string
	MailboxService      *service.MailboxService
	AccountService      *service.AccountService
	Logger              *log.Logger
}

type Handler struct {
	stripeWebhookSecret string
	mailboxService      *service.MailboxService
	accountService      *service.AccountService
	logger              *log.Logger
}

type accountContextKey struct{}

func NewHandler(cfg Config) *Handler {
	return &Handler{
		stripeWebhookSecret: cfg.StripeWebhookSecret,
		mailboxService:      cfg.MailboxService,
		accountService:      cfg.AccountService,
		logger:              cfg.Logger,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("POST /v1/accounts", h.handleCreateAccount)
	mux.HandleFunc("GET /v1/mailboxes", h.withAccountToken(h.handleListMailboxes))
	mux.HandleFunc("POST /v1/mailboxes", h.withAccountToken(h.handleCreateMailbox))
	mux.HandleFunc("GET /v1/mailboxes/{id}", h.withAccountToken(h.handleGetMailbox))
	mux.HandleFunc("POST /v1/imap/resolve", h.withAccountToken(h.handleResolveIMAP))
	mux.HandleFunc("POST /v1/imap/messages", h.withAccountToken(h.handleListIMAPMessages))
	mux.HandleFunc("POST /v1/webhooks/stripe", h.handleStripeWebhook)
	mux.HandleFunc("GET /mock/pay/{sessionID}", h.handleMockPay)
	return mux
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createAccountRequest struct {
	OwnerEmail string `json:"owner_email"`
}

type accountView struct {
	ID         string `json:"id"`
	OwnerEmail string `json:"owner_email"`
	APIToken   string `json:"api_token"`
	Created    bool   `json:"created"`
}

func (h *Handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	account, created, err := h.accountService.CreateOrGetAccount(r.Context(), req.OwnerEmail)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	writeJSON(w, status, accountView{
		ID:         account.ID,
		OwnerEmail: account.OwnerEmail,
		APIToken:   account.APIToken,
		Created:    created,
	})
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

func (h *Handler) handleResolveIMAP(w http.ResponseWriter, r *http.Request) {
	var req resolveIMAPRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result, err := h.mailboxService.ResolveIMAPByToken(r.Context(), req.AccessToken)
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

type listMessagesRequest struct {
	AccessToken string `json:"access_token"`
}

func (h *Handler) handleListIMAPMessages(w http.ResponseWriter, r *http.Request) {
	var req listMessagesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	_, err := h.mailboxService.ResolveIMAPByToken(r.Context(), req.AccessToken)
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
		"messages": []any{},
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
	AccessToken string               `json:"access_token,omitempty"`
}

func mailboxResponse(mailbox *domain.Mailbox) mailboxView {
	resp := mailboxView{
		ID:         mailbox.ID,
		Status:     mailbox.Status,
		Usable:     mailbox.Usable(),
		PaymentURL: mailbox.PaymentURL,
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
