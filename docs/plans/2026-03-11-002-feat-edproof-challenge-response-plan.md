---
title: "feat: Ed25519 challenge-response for edproof verification"
type: feat
status: active
date: 2026-03-11
---

# Ed25519 Challenge-Response for EdProof Verification

## Overview

Replace the passthrough `localBackend` in `internal/adapters/identity/edproof/verifier.go` with a stateless challenge-response flow that proves private key ownership before granting mailbox access.

## Problem Statement

The current `localBackend.Verify` accepts a public key and returns its fingerprint — no signature verification. Since public keys are, by definition, public, anyone who knows the key can access the mailbox. This is a real security gap.

## Proposed Solution

**Stateless challenge-response** with 30-second TTL:

1. Client calls `POST /v1/auth/challenge` with their public key
2. Server returns a challenge string: `v1.<unix_ts>.<hex_nonce>.<hmac_hex>`
3. Client signs the **raw challenge string bytes** (UTF-8) with their Ed25519 private key
4. Client calls claim/resolve with `{edproof, challenge, signature}` (all strings)
5. Server: verifies HMAC authenticity → checks timestamp ≤ 30s → verifies Ed25519 signature → extracts fingerprint

No server-side storage needed. HMAC proves server issued the challenge, timestamp proves freshness, Ed25519 signature proves private key ownership.

## Technical Considerations

### Challenge Format

```
v1.<unix_ts>.<hex_nonce>.<hmac_hex>
```

- `v1` — version prefix for future extensibility
- `unix_ts` — decimal seconds (string), used for TTL check
- `hex_nonce` — 16 random bytes hex-encoded (replay mitigation within TTL window)
- `hmac_hex` — `HMAC-SHA256(v1.<ts>.<nonce>.<canonical_pubkey>, secret)` hex-encoded

### Canonical Public Key in HMAC

Use only the base64 key blob (field 2 of `ssh-ed25519 AAAA... comment`), not the full line. This avoids comment-dependent HMAC differences.

### What the Client Signs

Raw challenge string as UTF-8 bytes — e.g. `ed25519.Sign(privkey, []byte("v1.1741234567.abcdef...1234..."))`. No decoding, no wrapping.

### SSH Key Parsing

Use `golang.org/x/crypto/ssh.ParsePublicKey` or manual base64 decode to extract the raw 32-byte Ed25519 public key for `ed25519.Verify`.

### Clock Skew

Reject challenges with timestamps more than 5 seconds in the future (client clock ahead).

### Security

- `EDPROOF_HMAC_SECRET` env var — required when edproof is enabled, fail at startup if empty
- Minimum 32 bytes, generated via `openssl rand -hex 32`
- Constant-time HMAC comparison via `hmac.Equal`
- Replay within 30s window mitigated by nonce (different challenge each request)

## Acceptance Criteria

- [x] `POST /v1/auth/challenge` returns a challenge given a valid SSH Ed25519 public key
- [x] Claim and resolve endpoints accept `challenge` + `signature` fields alongside `edproof`
- [x] Valid challenge + correct signature → access granted (fingerprint returned)
- [x] Missing challenge/signature → clear error message directing to challenge endpoint
- [x] Expired challenge (>30s) → rejected
- [x] Tampered challenge → rejected (HMAC mismatch)
- [x] Wrong private key → rejected (signature invalid)
- [x] Future timestamp (>5s ahead) → rejected
- [ ] `EDPROOF_HMAC_SECRET` missing at startup → fatal error
- [x] All existing tests updated, ≥80% coverage
- [x] `go test ./...` passes

## Implementation Phases

### Phase 1: Challenge generation and verification in `edproof` package

Files: `internal/adapters/identity/edproof/challenge.go`, `internal/adapters/identity/edproof/challenge_test.go`

- [x] Add `EDPROOF_HMAC_SECRET` to `config.go`
- [x] `GenerateChallenge(pubkey string, secret []byte, now time.Time) (string, error)`
- [x] `VerifyChallenge(challenge string, pubkey string, secret []byte, maxAge time.Duration, now time.Time) error`
- [x] `VerifySignature(challenge string, pubkey string, signatureB64 string) error`
- [x] Tests: happy path, expired, tampered HMAC, wrong pubkey in HMAC, future timestamp, wrong signature, malformed inputs (18 tests)

### Phase 2: Challenge HTTP endpoint

Files: `internal/adapters/httpapi/handler.go`

- [x] Add `POST /v1/auth/challenge` endpoint
- [x] Wire `EDPROOF_HMAC_SECRET` through `Handler` config
- [x] Tests for the endpoint: valid key, invalid key format, not configured (3 tests)

### Phase 3: Update claim and resolve to require challenge-response

Files: `internal/adapters/httpapi/handler.go`, `internal/adapters/identity/edproof/verifier.go`

- [x] Extend `claimMailboxRequest` and `resolveAccessRequest` with `Challenge` and `Signature` fields
- [x] Add `verifyEdproof()` method on Handler — challenge verification in handler layer, fingerprint via existing `KeyProofVerifier`
- [x] If challenge/signature empty → reject with error: `"edproof now requires challenge-response — call POST /v1/auth/challenge first"`
- [x] Existing handler tests pass (passthrough mode when no HMAC secret)
- [x] Integration tests: full claim flow, full resolve flow, missing challenge rejection, wrong signature rejection (4 tests)

### Phase 4: Config, wiring, deploy

- [x] Add `EDPROOF_HMAC_SECRET` to `.env.example`
- [x] Generate secret: `openssl rand -hex 32`
- [x] Add to GitHub secrets (`gh secret set EDPROOF_HMAC_SECRET --env production`)
- [x] Update `deploy-production.yml` to pass the secret
- [x] Update homepage/agent prompt text to document the new flow
- [ ] Smoke test: full challenge → sign → resolve on production

## Sources & References

### Internal References

- Current verifier: `internal/adapters/identity/edproof/verifier.go`
- Handler endpoints: `internal/adapters/httpapi/handler.go:594-725`
- Ports interface: `internal/core/ports/ports.go:107-109`
- Config: `internal/platform/config/config.go`
- Eidos plan: `memory/plan - 2603111027 - edproof challenge-response to verify private key ownership.md`

### External References

- `golang.org/x/crypto/ssh` — SSH public key parsing
- `crypto/ed25519` — Ed25519 signature verification
- `crypto/hmac` + `crypto/sha256` — HMAC challenge generation
