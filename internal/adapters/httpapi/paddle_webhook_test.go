package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	paddle "github.com/PaddleHQ/paddle-go-sdk/v5"
)

func TestVerifyPaddleWebhookAcceptsValidSignature(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)
	sig := signedPaddleHeader(secret, time.Now().Unix(), body)

	got, err := verifyPaddleWebhook(secret, sig, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("expected webhook verification to succeed, got %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("expected returned body to match input, got %q", got)
	}
}

func TestVerifyPaddleWebhookRejectsBadSignature(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)
	badSig := "ts=" + strconv.FormatInt(time.Now().Unix(), 10) + ";h1=" + strings.Repeat("0", 64)

	_, err := verifyPaddleWebhook(secret, badSig, strings.NewReader(string(body)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail")
	}
	if !errors.Is(err, errInvalidPaddleWebhook) {
		t.Fatalf("expected errInvalidPaddleWebhook, got %v", err)
	}
}

func TestVerifyPaddleWebhookRejectsStaleTimestamp(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)
	staleTS := time.Now().Add(-10 * time.Minute).Unix()
	sig := signedPaddleHeader(secret, staleTS, body)

	_, err := verifyPaddleWebhook(secret, sig, strings.NewReader(string(body)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail for stale timestamp")
	}
	if !errors.Is(err, paddle.ErrReplayAttack) {
		t.Fatalf("expected paddle.ErrReplayAttack, got %v", err)
	}
}

func TestVerifyPaddleWebhookRejectsMissingSignatureHeader(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)

	_, err := verifyPaddleWebhook(secret, "", strings.NewReader(string(body)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail for missing signature header")
	}
	if !errors.Is(err, paddle.ErrMissingSignature) {
		t.Fatalf("expected paddle.ErrMissingSignature, got %v", err)
	}
}

func TestVerifyPaddleWebhookRejectsMalformedSignatureHeader(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)

	_, err := verifyPaddleWebhook(secret, "not-a-valid-signature", strings.NewReader(string(body)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail for malformed signature header")
	}
	if !errors.Is(err, paddle.ErrInvalidSignatureFormat) {
		t.Fatalf("expected paddle.ErrInvalidSignatureFormat, got %v", err)
	}
}

func TestVerifyPaddleWebhookFailsClosedWhenSecretUnconfigured(t *testing.T) {
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_1"}}`)
	// Sign with some secret to prove an empty configured secret can't be
	// tricked into accepting a signature computed under any key.
	sig := signedPaddleHeader("pdl_ntfset_testsecret", time.Now().Unix(), body)

	_, err := verifyPaddleWebhook("", sig, strings.NewReader(string(body)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail when secret is unconfigured")
	}
	if !errors.Is(err, errPaddleWebhookSecretNotConfigured) {
		t.Fatalf("expected errPaddleWebhookSecretNotConfigured, got %v", err)
	}
}

func TestVerifyPaddleWebhookRejectsBodyExceedingSizeCap(t *testing.T) {
	secret := "pdl_ntfset_testsecret"
	oversized := []byte(strings.Repeat("a", maxRequestBodyBytes+1))
	// Sign the full oversized body. If the cap were not applied before the
	// verifier read the body, this signature would still verify. Since we
	// expect the cap to truncate the body first, verification must fail.
	sig := signedPaddleHeader(secret, time.Now().Unix(), oversized)

	_, err := verifyPaddleWebhook(secret, sig, strings.NewReader(string(oversized)))
	if err == nil {
		t.Fatalf("expected webhook verification to fail for oversized body")
	}
	if !errors.Is(err, errInvalidPaddleWebhook) {
		t.Fatalf("expected errInvalidPaddleWebhook (signature mismatch on truncated body), got %v", err)
	}
}

func signedPaddleHeader(secret string, ts int64, body []byte) string {
	tsStr := strconv.FormatInt(ts, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte(":"))
	mac.Write(body)
	return "ts=" + tsStr + ";h1=" + hex.EncodeToString(mac.Sum(nil))
}
