package main

import (
	"context"
	"log"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/adapters/payment"
	"github.com/atvirokodosprendimai/mailservice/internal/platform/config"
)

func TestNewKeyProofVerifierUsesLocalVerifier(t *testing.T) {
	t.Parallel()

	verifier := newKeyProofVerifier()
	if verifier == nil {
		t.Fatal("expected verifier")
	}

	key, err := verifier.Verify(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOB0H114bTlib+M0AuEoXJDWHzU52aMKtT8O1wtpk5WB entity@context")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if key.Fingerprint != "sha256:ac1f9a7583aea966a3320d0124329b982a0dd4aa95bf22f598ee089aea6357c0" {
		t.Fatalf("unexpected fingerprint %q", key.Fingerprint)
	}
}

func TestSelectPaymentGatewayNoProvidersConfigured(t *testing.T) {
	t.Parallel()

	gateway, mockMode := selectPaymentGateway(&config.Config{PublicBaseURL: "http://localhost:8080"}, log.Default(), nil)
	if _, ok := gateway.(*payment.MockGateway); !ok {
		t.Fatalf("expected MockGateway, got %T", gateway)
	}
	if !mockMode {
		t.Fatalf("expected mockPaymentMode=true, got false")
	}
}

func TestSelectPaymentGatewayStripeOnly(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		StripeSecretKey: "sk_test_123",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default(), nil)
	if _, ok := gateway.(*payment.StripeGateway); !ok {
		t.Fatalf("expected StripeGateway, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}

func TestSelectPaymentGatewayPaddlePreferredOverStripe(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		PaddleAPIKey:      "paddle-key",
		PaddlePriceID:     "pri_123",
		PaddleEnvironment: "sandbox",
		StripeSecretKey:   "sk_test_123",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default(), nil)
	if _, ok := gateway.(*payment.PaddleGateway); !ok {
		t.Fatalf("expected PaddleGateway, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}

func TestSelectGiftCouponConfigPaddleActiveWithGiftConfigured(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		PaddleAPIKey:         "paddle-key",
		PaddlePriceID:        "pri_123",
		PaddleGiftDiscountID: "dsc_paddle",
		PaddleGiftCouponCode: "PADDLECODE",
	}
	giftOpts := selectGiftCouponConfig(cfg, log.Default())
	if len(giftOpts) != 1 {
		t.Fatalf("expected exactly one gift coupon config, got %d", len(giftOpts))
	}
	if giftOpts[0].DiscountID != "dsc_paddle" || giftOpts[0].CouponCode != "PADDLECODE" {
		t.Fatalf("expected paddle gift coupon config, got %+v", giftOpts[0])
	}
}

func TestSelectGiftCouponConfigPaddleActiveButGiftIncompleteDisablesCoupons(t *testing.T) {
	t.Parallel()

	// Paddle is the active gateway but only half the Paddle gift env vars are
	// set: coupons must be disabled, not silently half-applied.
	cfg := &config.Config{
		PaddleAPIKey:         "paddle-key",
		PaddlePriceID:        "pri_123",
		PaddleGiftDiscountID: "dsc_paddle",
	}
	giftOpts := selectGiftCouponConfig(cfg, log.Default())
	if len(giftOpts) != 0 {
		t.Fatalf("expected no gift coupon config, got %+v", giftOpts)
	}
}

func TestSelectGiftCouponConfigNoneConfigured(t *testing.T) {
	t.Parallel()

	giftOpts := selectGiftCouponConfig(&config.Config{}, log.Default())
	if len(giftOpts) != 0 {
		t.Fatalf("expected no gift coupon config, got %+v", giftOpts)
	}
}
