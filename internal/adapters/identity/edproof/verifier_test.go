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

func TestVerifierReturnsInvalidProofFromBackendWithoutWrapping(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(fakeBackend{err: ports.ErrInvalidKeyProof})

	_, err := verifier.Verify(context.Background(), "proof")
	if !errors.Is(err, ports.ErrInvalidKeyProof) {
		t.Fatalf("expected ErrInvalidKeyProof, got %v", err)
	}
	if err.Error() != ports.ErrInvalidKeyProof.Error() {
		t.Fatalf("expected canonical invalid key proof error, got %q", err.Error())
	}
}

func TestVerifierFallsBackToLocalSSHVerifier(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(nil)

	key, err := verifier.Verify(context.Background(), "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOB0H114bTlib+M0AuEoXJDWHzU52aMKtT8O1wtpk5WB entity@context")
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if key.Fingerprint != "sha256:ac1f9a7583aea966a3320d0124329b982a0dd4aa95bf22f598ee089aea6357c0" {
		t.Fatalf("expected local verifier fingerprint, got %q", key.Fingerprint)
	}
	if key.Algorithm != "ed25519" {
		t.Fatalf("expected ed25519 algorithm, got %q", key.Algorithm)
	}
}

func TestVerifierReturnsInvalidProofWithoutWrapping(t *testing.T) {
	t.Parallel()

	verifier := NewVerifier(nil)

	_, err := verifier.Verify(context.Background(), "ssh-ed25519 !!!not-base64!!! entity@context")
	if !errors.Is(err, ports.ErrInvalidKeyProof) {
		t.Fatalf("expected ErrInvalidKeyProof, got %v", err)
	}
	if err.Error() != ports.ErrInvalidKeyProof.Error() {
		t.Fatalf("expected canonical invalid key proof error, got %q", err.Error())
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
