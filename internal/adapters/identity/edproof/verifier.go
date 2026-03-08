package edproof

import (
	"context"
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
