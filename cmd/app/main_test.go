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

	gateway, mockMode := selectPaymentGateway(&config.Config{PublicBaseURL: "http://localhost:8080"}, log.Default())
	if _, ok := gateway.(*payment.MockGateway); !ok {
		t.Fatalf("expected MockGateway, got %T", gateway)
	}
	if !mockMode {
		t.Fatalf("expected mockPaymentMode=true, got false")
	}
}

func TestSelectPaymentGatewayPolarOnly(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		PolarToken:     "polar-token",
		PolarProductID: "prod_123",
		PolarServerURL: "https://api.polar.sh",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default())
	if _, ok := gateway.(*payment.PolarGateway); !ok {
		t.Fatalf("expected PolarGateway, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}

func TestSelectPaymentGatewayStripeOnly(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		StripeSecretKey: "sk_test_123",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default())
	if _, ok := gateway.(*payment.StripeGateway); !ok {
		t.Fatalf("expected StripeGateway, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}

func TestSelectPaymentGatewayPaddlePreferredOverPolarAndStripe(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		PaddleAPIKey:      "paddle-key",
		PaddlePriceID:     "pri_123",
		PaddleEnvironment: "sandbox",
		PolarToken:        "polar-token",
		PolarProductID:    "prod_123",
		StripeSecretKey:   "sk_test_123",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default())
	if _, ok := gateway.(*payment.PaddleGateway); !ok {
		t.Fatalf("expected PaddleGateway, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}

func TestSelectPaymentGatewayPaddleAbsentLeavesPolarAndStripeUnaffected(t *testing.T) {
	t.Parallel()

	// No Paddle config at all: Polar must still win over Stripe, exactly as
	// before Paddle support was added.
	cfg := &config.Config{
		PolarToken:      "polar-token",
		PolarProductID:  "prod_123",
		StripeSecretKey: "sk_test_123",
	}
	gateway, mockMode := selectPaymentGateway(cfg, log.Default())
	if _, ok := gateway.(*payment.PolarGateway); !ok {
		t.Fatalf("expected PolarGateway when Paddle absent, got %T", gateway)
	}
	if mockMode {
		t.Fatalf("expected mockPaymentMode=false, got true")
	}
}
