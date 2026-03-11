package edproof

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
		if errors.Is(err, ports.ErrInvalidKeyProof) {
			return nil, ports.ErrInvalidKeyProof
		}
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

// FingerprintFromPubkey computes the sha256:<hex> fingerprint from an SSH public key line
// (e.g. "ssh-ed25519 AAAA... comment").
func FingerprintFromPubkey(pubkey string) (string, error) {
	parts := strings.Fields(pubkey)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid public key format")
	}
	if parts[0] != "ssh-ed25519" {
		return "", fmt.Errorf("unsupported key type %q, expected ssh-ed25519", parts[0])
	}
	keyBlob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid base64 in public key: %w", err)
	}
	hash := sha256.Sum256(keyBlob)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

type localBackend struct{}

func (localBackend) Verify(_ context.Context, rawProof string) (*BackendResult, error) {
	fingerprint, err := FingerprintFromPubkey(rawProof)
	if err != nil {
		return nil, ports.ErrInvalidKeyProof
	}
	return &BackendResult{
		Fingerprint: fingerprint,
		Algorithm:   "ed25519",
	}, nil
}
