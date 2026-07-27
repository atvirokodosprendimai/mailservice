package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"
	"github.com/PaddleHQ/paddle-go-sdk/v5/pkg/paddleerr"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/metrics"
)

// errPaddleMissingCheckoutURL is returned when Paddle accepts a transaction
// create request but the response has no checkout.url, which would otherwise
// silently produce a PaymentLink with an empty URL.
var errPaddleMissingCheckoutURL = errors.New("paddle: checkout url missing from transaction response")

// paddleCheckoutPagePath is U6's own checkout page, which Paddle.js opens
// client-side via the transaction ID passed as _ptxn — Paddle's documented
// convention for auto-resuming an existing transaction's overlay.
const paddleCheckoutPagePath = "/v1/payments/paddle/checkout"

type PaddleConfig struct {
	BaseURL string
	APIKey  string
	PriceID string
	// CheckoutBaseURL is this app's own public base URL (e.g.
	// https://truevipaccess.com). When set, CreatePaymentLink returns a link
	// to U6's own checkout page (which server-side validates the session and
	// renders Paddle.js) instead of Paddle's raw hosted checkout.url. Left
	// empty, it falls back to Paddle's checkout.url directly.
	CheckoutBaseURL string
	Client          *http.Client
	// Metrics is the shared metrics registry counters are recorded on. Nil
	// is safe (Registry's methods no-op on a nil receiver) so tests that
	// don't care about metrics can omit it.
	Metrics *metrics.Registry
}

type PaddleGateway struct {
	sdk             *paddle.SDK
	priceID         string
	checkoutBaseURL string
	metrics         *metrics.Registry
}

func NewPaddleGateway(cfg PaddleConfig) (*PaddleGateway, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = paddle.ProductionBaseURL
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	sdk, err := paddle.New(strings.TrimSpace(cfg.APIKey), paddle.WithBaseURL(baseURL), paddle.WithClient(client))
	if err != nil {
		return nil, fmt.Errorf("paddle sdk init: %w", err)
	}

	return &PaddleGateway{
		sdk:             sdk,
		priceID:         strings.TrimSpace(cfg.PriceID),
		checkoutBaseURL: strings.TrimRight(strings.TrimSpace(cfg.CheckoutBaseURL), "/"),
		metrics:         cfg.Metrics,
	}, nil
}

func (g *PaddleGateway) CreatePaymentLink(ctx context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	// Checkout is intentionally left unset on the request: checkout.url comes
	// from Paddle's account-level default payment link (provisioned by U9),
	// not from app config, so it can't drift from the account's true default.
	createReq := &paddle.CreateTransactionRequest{
		Items: []paddle.CreateTransactionItems{
			*paddle.NewCreateTransactionItemsTransactionItemFromCatalog(&paddle.TransactionItemFromCatalog{
				PriceID:  g.priceID,
				Quantity: 1,
			}),
		},
		CustomData: paddle.CustomData{
			"mailbox_id":  req.MailboxID,
			"owner_email": req.OwnerEmail,
		},
	}
	if req.DiscountID != "" {
		discountID := req.DiscountID
		createReq.DiscountID = &discountID
	}

	txn, err := g.sdk.CreateTransaction(ctx, createReq)
	if err != nil {
		if req.DiscountID != "" {
			if mapped := mapPaddleDiscountError(err); mapped != nil {
				return nil, fmt.Errorf("%w: %v", mapped, err)
			}
		}
		return nil, fmt.Errorf("paddle create transaction: %w", err)
	}

	if req.DiscountID != "" && !discountApplied(txn) {
		return nil, fmt.Errorf("%w: paddle accepted discount_id %q but did not apply it", ports.ErrCouponInvalid, req.DiscountID)
	}

	if txn.Checkout == nil || txn.Checkout.URL == nil || *txn.Checkout.URL == "" {
		return nil, fmt.Errorf("%w: transaction %s", errPaddleMissingCheckoutURL, txn.ID)
	}

	checkoutURL := *txn.Checkout.URL
	if g.checkoutBaseURL != "" {
		checkoutURL = g.checkoutBaseURL + paddleCheckoutPagePath + "?_ptxn=" + url.QueryEscape(txn.ID)
	}

	g.metrics.Counter("payment_link_created").Add(1)
	return &ports.PaymentLink{
		SessionID: txn.ID,
		URL:       checkoutURL,
	}, nil
}

func (g *PaddleGateway) GetPaymentSession(ctx context.Context, sessionID string) (*ports.PaymentSession, error) {
	g.metrics.Counter("payment_session_lookup").Add(1)

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("paddle get transaction: missing session id")
	}

	txn, err := g.sdk.GetTransaction(ctx, &paddle.GetTransactionRequest{TransactionID: sessionID})
	if err != nil {
		var apiErr *paddleerr.Error
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, ports.ErrPaymentSessionNotFound
		}
		return nil, fmt.Errorf("paddle get transaction: %w", err)
	}

	var checkoutURL string
	if txn.Checkout != nil && txn.Checkout.URL != nil {
		checkoutURL = *txn.Checkout.URL
	}

	return &ports.PaymentSession{
		SessionID: txn.ID,
		Status:    mapPaddleStatus(txn.Status),
		URL:       checkoutURL,
	}, nil
}

// discountApplied reports whether Paddle actually applied a positive discount
// to the transaction totals. Paddle can accept a discount_id on create and
// still return a transaction where the discount total is zero (e.g. the
// discount doesn't apply to the price being purchased); this catches that
// silent-acceptance case so it isn't mistaken for success.
func discountApplied(txn *paddle.Transaction) bool {
	discount := strings.TrimSpace(txn.Details.Totals.Discount)
	if discount == "" {
		return false
	}
	amount, err := strconv.ParseFloat(discount, 64)
	if err != nil {
		return false
	}
	return amount > 0
}

// Substrings of Paddle's error.code that indicate a discount was rejected as
// exhausted/expired vs. invalid/inapplicable. Named here (rather than inlined
// in mapPaddleDiscountError) so the exact-code list can be tightened to a
// precise switch once live sandbox responses confirm Paddle's actual codes —
// see the KNOWN GAP note below. Sourced from the paddle-go-sdk's documented
// sentinel codes (transaction_discount_not_found,
// transaction_discount_not_eligible, discount_expired,
// discount_usage_limit_exceeded) as of U2/U7; NOT verified against a live
// Paddle API call.
const (
	paddleDiscountCodeSubstringExpired    = "expired"
	paddleDiscountCodeSubstringUsageLimit = "usage_limit"
	paddleDiscountCodeSubstringDiscount   = "discount"
)

// mapPaddleDiscountError inspects Paddle's structured error code and returns
// the matching ports coupon sentinel, or nil if err isn't a discount-related
// Paddle API error.
//
// KNOWN GAP: the substring matches below are a best-effort reading of
// Paddle's documented error codes, not a live-verified exact list. Before
// this handles production traffic, make sandbox calls with an
// exhausted/expired/invalid discount and a valid-but-inapplicable discount,
// record the exact error.code/error.type Paddle returns, and replace this
// with an exact switch over confirmed codes.
func mapPaddleDiscountError(err error) error {
	var apiErr *paddleerr.Error
	if !errors.As(err, &apiErr) {
		return nil
	}
	code := apiErr.Code
	switch {
	case strings.Contains(code, paddleDiscountCodeSubstringExpired):
		return ports.ErrCouponExhausted
	case strings.Contains(code, paddleDiscountCodeSubstringUsageLimit):
		return ports.ErrCouponExhausted
	case strings.Contains(code, paddleDiscountCodeSubstringDiscount):
		return ports.ErrCouponInvalid
	default:
		return nil
	}
}

func mapPaddleStatus(status paddle.TransactionStatus) ports.PaymentSessionStatus {
	switch status {
	case paddle.TransactionStatusDraft, paddle.TransactionStatusReady:
		return ports.PaymentSessionStatusOpen
	case paddle.TransactionStatusBilled, paddle.TransactionStatusPaid, paddle.TransactionStatusCompleted:
		return ports.PaymentSessionStatusSucceeded
	case paddle.TransactionStatusCanceled:
		return ports.PaymentSessionStatusFailed
	default:
		return ports.PaymentSessionStatusOpen
	}
}
