package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"
	"github.com/PaddleHQ/paddle-go-sdk/v5/pkg/paddleerr"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

// errPaddleMissingCheckoutURL is returned when Paddle accepts a transaction
// create request but the response has no checkout.url, which would otherwise
// silently produce a PaymentLink with an empty URL.
var errPaddleMissingCheckoutURL = errors.New("paddle: checkout url missing from transaction response")

type PaddleConfig struct {
	BaseURL               string
	APIKey                string
	PriceID               string
	DefaultPaymentLinkURL string
	Client                *http.Client
}

type PaddleGateway struct {
	sdk                   *paddle.SDK
	priceID               string
	defaultPaymentLinkURL string
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
		sdk:                   sdk,
		priceID:               strings.TrimSpace(cfg.PriceID),
		defaultPaymentLinkURL: strings.TrimSpace(cfg.DefaultPaymentLinkURL),
	}, nil
}

func (g *PaddleGateway) CreatePaymentLink(ctx context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
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
	if g.defaultPaymentLinkURL != "" {
		checkoutURL := g.defaultPaymentLinkURL
		createReq.Checkout = &paddle.TransactionCheckout{URL: &checkoutURL}
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

	return &ports.PaymentLink{
		SessionID: txn.ID,
		URL:       *txn.Checkout.URL,
	}, nil
}

func (g *PaddleGateway) GetPaymentSession(ctx context.Context, sessionID string) (*ports.PaymentSession, error) {
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

// mapPaddleDiscountError inspects Paddle's structured error code and returns
// the matching ports coupon sentinel, or nil if err isn't a discount-related
// Paddle API error.
func mapPaddleDiscountError(err error) error {
	var apiErr *paddleerr.Error
	if !errors.As(err, &apiErr) {
		return nil
	}
	code := apiErr.Code
	switch {
	case strings.Contains(code, "expired"):
		return ports.ErrCouponExhausted
	case strings.Contains(code, "usage_limit"):
		return ports.ErrCouponExhausted
	case strings.Contains(code, "discount"):
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
