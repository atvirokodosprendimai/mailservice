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
	if key.Fingerprint != "sha256:ac1f9a7583aea966a3320d0124329b982a0dd4aa95bf22f598ee089aea6357c0" {
		t.Fatalf("unexpected fingerprint %q", key.Fingerprint)
	}
}
