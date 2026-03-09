package main

import (
	"context"
	"testing"
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
	if key.Fingerprint != "sha256:rb+adyouqwajmg0bjdkbmcon1kqvvyl1mo4imupjv8a" {
		t.Fatalf("unexpected fingerprint %q", key.Fingerprint)
	}
}
