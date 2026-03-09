package edproof

import (
	"context"
	"errors"
	"testing"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

func TestVerifierNormalizesVerifiedKey(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(fakeBackend{
		result: &BackendResult{
			Fingerprint: "  EDPROOF:ABC123  ",
			Algorithm:   "  Ed25519  ",
		},
	})

	key, err := verifier.Verify(context.Background(), "proof")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if key.Fingerprint != "edproof:abc123" {
		t.Fatalf("expected normalized fingerprint, got %q", key.Fingerprint)
	}
	if key.Algorithm != "ed25519" {
		t.Fatalf("expected normalized algorithm, got %q", key.Algorithm)
	}
}

func TestVerifierRejectsEmptyProof(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(fakeBackend{})

	_, err := verifier.Verify(context.Background(), "  ")
	if !errors.Is(err, ports.ErrInvalidKeyProof) {
		t.Fatalf("expected ErrInvalidKeyProof, got %v", err)
	}
}

func TestVerifierReturnsBackendError(t *testing.T) {
	t.Parallel()

	backendErr := errors.New("boom")
	verifier := NewVerifier(fakeBackend{err: backendErr})

	_, err := verifier.Verify(context.Background(), "proof")
	if !errors.Is(err, backendErr) {
		t.Fatalf("expected backend error, got %v", err)
	}
}

func TestVerifierRequiresBackend(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(nil)

	key, err := verifier.Verify(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOB0H114bTlib+M0AuEoXJDWHzU52aMKtT8O1wtpk5WB entity@context")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if key.Fingerprint != "sha256:rb+adyouqwajmg0bjdkbmcon1kqvvyl1mo4imupjv8a" {
		t.Fatalf("expected local verifier fingerprint, got %q", key.Fingerprint)
	}
	if key.Algorithm != "ed25519" {
		t.Fatalf("expected ed25519 algorithm, got %q", key.Algorithm)
	}
}

type fakeBackend struct {
	result *BackendResult
	err    error
}

func (f fakeBackend) Verify(_ context.Context, _ string) (*BackendResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}
