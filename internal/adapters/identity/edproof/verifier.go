package edproof

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

var ErrVerifierNotConfigured = errors.New("edproof verifier not configured")

type BackendResult struct {
	Fingerprint string
	Algorithm   string
}

type Backend interface {
	Verify(ctx context.Context, rawProof string) (*BackendResult, error)
}

type Verifier struct {
	backend Backend
}

func NewVerifier(backend Backend) *Verifier {
	if backend == nil {
		backend = localBackend{}
	}
	return &Verifier{backend: backend}
}

func (v *Verifier) Verify(ctx context.Context, rawProof string) (*ports.VerifiedKey, error) {
	rawProof = strings.TrimSpace(rawProof)
	if rawProof == "" {
		return nil, ports.ErrInvalidKeyProof
	}
	if v.backend == nil {
		return nil, ErrVerifierNotConfigured
	}

	result, err := v.backend.Verify(ctx, rawProof)
	if err != nil {
		return nil, fmt.Errorf("verify edproof: %w", err)
	}
	if result == nil {
		return nil, ports.ErrInvalidKeyProof
	}

	key := &ports.VerifiedKey{
		Fingerprint: strings.TrimSpace(strings.ToLower(result.Fingerprint)),
		Algorithm:   strings.TrimSpace(strings.ToLower(result.Algorithm)),
	}
	if key.Fingerprint == "" || key.Algorithm == "" {
		return nil, ports.ErrInvalidKeyProof
	}
	return key, nil
}

type localBackend struct{}

func (localBackend) Verify(_ context.Context, rawProof string) (*BackendResult, error) {
	parts := strings.Fields(rawProof)
	if len(parts) < 2 {
		return nil, ports.ErrInvalidKeyProof
	}
	if parts[0] != "ssh-ed25519" {
		return nil, ports.ErrInvalidKeyProof
	}

	keyBlob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ports.ErrInvalidKeyProof
	}

	fingerprint := sha256.Sum256(keyBlob)
	return &BackendResult{
		Fingerprint: "SHA256:" + base64.StdEncoding.WithPadding(base64.NoPadding).EncodeToString(fingerprint[:]),
		Algorithm:   "ed25519",
	}, nil
}
