package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"
	"github.com/PaddleHQ/paddle-go-sdk/v5/pkg/paddlenotification"
)

// paddleWebhookTolerance matches CONSTITUTION.md SEC-005's 5-minute
// timestamp freshness requirement. The SDK has no default tolerance — it
// only enforces one when explicitly configured via VerifierWithTimestampTolerance.
const paddleWebhookTolerance = 5 * time.Minute

var (
	errPaddleWebhookSecretNotConfigured = errors.New("paddle webhook secret not configured")
	errInvalidPaddleWebhook             = errors.New("invalid paddle webhook signature")
)

// verifyPaddleWebhook reads bodyReader (capped at maxRequestBodyBytes, applied
// before the SDK verifier ever sees the body) and verifies it against the
// Paddle-Signature header using the Paddle Go SDK's webhook verifier. It
// returns the verified body so callers can parse it without re-reading the
// request.
func verifyPaddleWebhook(secret string, signatureHeader string, bodyReader io.Reader) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errPaddleWebhookSecretNotConfigured
	}

	body, err := io.ReadAll(io.LimitReader(bodyReader, maxRequestBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read paddle webhook body: %w", err)
	}

	verifier := paddle.NewWebhookVerifier(secret, paddle.VerifierWithTimestampTolerance(paddleWebhookTolerance))

	req := &http.Request{Header: http.Header{}}
	req.Header.Set("Paddle-Signature", signatureHeader)
	req.Body = io.NopCloser(bytes.NewReader(body))

	ok, err := verifier.Verify(req)
	if err != nil {
		return nil, fmt.Errorf("verify paddle webhook: %w", err)
	}
	if !ok {
		return nil, errInvalidPaddleWebhook
	}

	return body, nil
}

var errInvalidPaddleWebhookPayload = errors.New("invalid paddle webhook payload")

// paddlePaymentEvent is the subset of a verified Paddle webhook payload the
// handler acts on, pre-extracted from Paddle's "data" shape so the routing
// logic doesn't need to parse JSON itself.
type paddlePaymentEvent struct {
	EventID    string
	EventType  paddlenotification.EventTypeName
	OccurredAt time.Time

	// SubscriptionID is the Paddle subscription ID this event concerns, used
	// as the primary mailbox join key (R4). Empty when the event carries
	// none, e.g. a transaction.completed for a one-off non-subscription
	// charge.
	SubscriptionID string
	// MailboxID is custom_data.mailbox_id, the fallback join key used when
	// no mailbox has this subscription_id on record yet (first payment,
	// before the mailbox's subscription_id column is populated).
	MailboxID string
	// BillingPeriodEndsAt is the end of the billing period this event
	// concerns: for transaction.* events, the transaction's billing_period
	// (set by Paddle for subscription charges, at the top level of the
	// transaction, not per line item); for subscription.* events, the
	// subscription's current_billing_period. Nil when absent.
	BillingPeriodEndsAt *time.Time
}

// paddleWebhookData is a shape-compatible superset of the "data" object
// across Paddle's subscription.* and transaction.* notifications: both
// carry id/custom_data, transactions additionally carry subscription_id and
// billing_period, subscriptions carry current_billing_period instead. Using
// one generic struct (rather than a typed struct per event_type) means
// mailbox resolution and dedup/ordering work uniformly for any event Paddle
// sends — including event types this handler doesn't explicitly route —
// so an unrecognized event_type is rejected by the routing switch, not by
// an accidental resolution failure.
type paddleWebhookData struct {
	ID                   string                         `json:"id"`
	SubscriptionID       *string                        `json:"subscription_id"`
	CustomData           paddlenotification.CustomData  `json:"custom_data"`
	BillingPeriod        *paddlenotification.TimePeriod `json:"billing_period"`
	CurrentBillingPeriod *paddlenotification.TimePeriod `json:"current_billing_period"`
}

type paddleWebhookEnvelope struct {
	EventID    string                           `json:"event_id"`
	EventType  paddlenotification.EventTypeName `json:"event_type"`
	OccurredAt string                           `json:"occurred_at"`
	Data       paddleWebhookData                `json:"data"`
}

// parsePaddleWebhookEvent parses a verified Paddle webhook body into a
// paddlePaymentEvent.
func parsePaddleWebhookEvent(body []byte) (*paddlePaymentEvent, error) {
	var envelope paddleWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse paddle webhook: %w", err)
	}
	if strings.TrimSpace(string(envelope.EventType)) == "" {
		return nil, errInvalidPaddleWebhookPayload
	}

	// occurred_at drives the ordering/dedup guard, so a missing or
	// malformed value must fail loud (reject the webhook) rather than
	// silently default to the zero time — Paddle guarantees this field on
	// every real event.
	occurredAt, err := time.Parse(time.RFC3339, envelope.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("parse paddle webhook occurred_at %q: %w", envelope.OccurredAt, err)
	}

	event := &paddlePaymentEvent{
		EventID:    strings.TrimSpace(envelope.EventID),
		EventType:  envelope.EventType,
		OccurredAt: occurredAt.UTC(),
		MailboxID:  paddleCustomDataMailboxID(envelope.Data.CustomData),
	}

	isSubscriptionEvent := strings.HasPrefix(string(envelope.EventType), "subscription.")
	isTransactionEvent := strings.HasPrefix(string(envelope.EventType), "transaction.")

	switch {
	case isSubscriptionEvent:
		// A subscription notification's own "id" *is* the subscription ID.
		event.SubscriptionID = strings.TrimSpace(envelope.Data.ID)
		event.BillingPeriodEndsAt = paddleTimePeriodEnd(envelope.Data.CurrentBillingPeriod)
	case isTransactionEvent:
		if envelope.Data.SubscriptionID != nil {
			event.SubscriptionID = strings.TrimSpace(*envelope.Data.SubscriptionID)
		}
		event.BillingPeriodEndsAt = paddleTimePeriodEnd(envelope.Data.BillingPeriod)
	}

	return event, nil
}

func paddleCustomDataMailboxID(data paddlenotification.CustomData) string {
	if data == nil {
		return ""
	}
	v, ok := data["mailbox_id"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func paddleTimePeriodEnd(period *paddlenotification.TimePeriod) *time.Time {
	if period == nil || strings.TrimSpace(period.EndsAt) == "" {
		return nil
	}
	endsAt, err := time.Parse(time.RFC3339, period.EndsAt)
	if err != nil {
		return nil
	}
	endsAt = endsAt.UTC()
	return &endsAt
}
