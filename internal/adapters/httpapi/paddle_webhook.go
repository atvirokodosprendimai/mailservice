package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"
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
