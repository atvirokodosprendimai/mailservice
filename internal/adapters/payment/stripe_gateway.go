package payment

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/checkout/session"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

type StripeConfig struct {
	SecretKey  string
	PriceCents int64
	Currency   string
	SuccessURL string
	CancelURL  string
}

type StripeGateway struct {
	priceCents int64
	currency   string
	successURL string
	cancelURL  string
}

func NewStripeGateway(cfg StripeConfig) *StripeGateway {
	stripe.Key = cfg.SecretKey
	return &StripeGateway{
		priceCents: cfg.PriceCents,
		currency:   cfg.Currency,
		successURL: cfg.SuccessURL,
		cancelURL:  cfg.CancelURL,
	}
}

func (g *StripeGateway) CreatePaymentLink(ctx context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	if req.DiscountID != "" {
		return nil, fmt.Errorf("discount codes not supported with Stripe gateway")
	}
	params := &stripe.CheckoutSessionParams{
		Params:        stripe.Params{Context: ctx},
		Mode:          stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:    stripe.String(g.successURL),
		CancelURL:     stripe.String(g.cancelURL),
		CustomerEmail: stripe.String(req.OwnerEmail),
		Metadata: map[string]string{
			"mailbox_id":  req.MailboxID,
			"owner_email": req.OwnerEmail,
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(g.currency),
					UnitAmount: stripe.Int64(g.priceCents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String("Mailbox activation"),
						Description: stripe.String(fmt.Sprintf("Activate mailbox %s", req.MailboxID)),
					},
				},
			},
		},
	}

	sess, err := session.New(params)
	if err != nil {
		return nil, err
	}

	return &ports.PaymentLink{
		SessionID: sess.ID,
		URL:       sess.URL,
	}, nil
}

func (g *StripeGateway) GetPaymentSession(ctx context.Context, sessionID string) (*ports.PaymentSession, error) {
	sess, err := session.Get(sessionID, &stripe.CheckoutSessionParams{
		Params: stripe.Params{Context: ctx},
	})
	if err != nil {
		return nil, err
	}

	status := ports.PaymentSessionStatusOpen
	switch sess.PaymentStatus {
	case stripe.CheckoutSessionPaymentStatusPaid:
		status = ports.PaymentSessionStatusSucceeded
	case stripe.CheckoutSessionPaymentStatusNoPaymentRequired:
		status = ports.PaymentSessionStatusConfirmed
	case stripe.CheckoutSessionPaymentStatusUnpaid:
		status = ports.PaymentSessionStatusOpen
	}

	return &ports.PaymentSession{
		SessionID: sess.ID,
		Status:    status,
		URL:       sess.URL,
	}, nil
}

type MockGateway struct {
	baseURL string
}

func NewMockGateway(baseURL string) *MockGateway {
	return &MockGateway{baseURL: baseURL}
}

func (g *MockGateway) CreatePaymentLink(_ context.Context, req ports.PaymentLinkRequest) (*ports.PaymentLink, error) {
	sessionID := "mock_" + uuid.NewString()
	// DiscountID accepted and ignored — Polar-only feature.
	return &ports.PaymentLink{
		SessionID: sessionID,
		URL:       fmt.Sprintf("%s/mock/pay/%s", g.baseURL, sessionID),
	}, nil
}

func (g *MockGateway) GetPaymentSession(_ context.Context, sessionID string) (*ports.PaymentSession, error) {
	return &ports.PaymentSession{
		SessionID: sessionID,
		Status:    ports.PaymentSessionStatusSucceeded,
		URL:       fmt.Sprintf("%s/mock/pay/%s", g.baseURL, sessionID),
	}, nil
}
