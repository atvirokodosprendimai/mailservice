---
tldr: Replace passthrough edproof with stateless challenge-response that proves private key ownership
status: active
---

# Plan: Ed25519 challenge-response for edproof

## Context

The `localBackend` in `internal/adapters/identity/edproof/verifier.go` accepts a public key and returns the fingerprint without verifying the caller holds the private key.
Anyone who knows the public key can access the mailbox.
Public keys are, by definition, public — this is a real security gap.

## Design

**Stateless challenge-response** with 30-second TTL:

1. Client calls `POST /v1/auth/challenge` with their public key
2. Server returns a challenge = `base64(HMAC-SHA256(pubkey + timestamp, server_secret) || timestamp)`
3. Client signs the challenge with their Ed25519 private key
4. Client calls claim/resolve with `{"edproof": "<pubkey>", "challenge": "<challenge>", "signature": "<base64-sig>"}`
5. Server: (a) verifies challenge authenticity via HMAC, (b) checks timestamp within 30s, (c) verifies Ed25519 signature of challenge bytes against pubkey, (d) extracts fingerprint

No server-side storage needed. The HMAC proves the server issued the challenge, the timestamp proves freshness, the Ed25519 signature proves private key ownership.

**Server secret**: `EDPROOF_HMAC_SECRET` env var. Required when edproof is enabled.

## Phases

### Phase 1 - Challenge endpoint and signing backend - status: open

1. [ ] Add `EDPROOF_HMAC_SECRET` to config
   - new field in Config struct
   - required when used (fail at startup if empty and edproof is needed)
2. [ ] Implement challenge generation in `edproof` package
   - `GenerateChallenge(pubkey string, secret []byte) (string, error)`
   - challenge = `base64(hmac_bytes || timestamp_bytes)`
   - timestamp is unix seconds as 8-byte big-endian
3. [ ] Implement challenge verification in `edproof` package
   - `VerifyChallenge(challenge string, pubkey string, secret []byte, maxAge time.Duration) error`
   - recompute HMAC, compare, check timestamp
4. [ ] Implement Ed25519 signature verification
   - parse SSH public key → extract raw 32-byte Ed25519 key
   - `ed25519.Verify(pubkey, challengeBytes, signature)`
5. [ ] Add `POST /v1/auth/challenge` endpoint
   - request: `{"public_key": "ssh-ed25519 AAAA..."}`
   - response: `{"challenge": "<base64>", "expires_in": 30}`
6. [ ] Replace `localBackend.Verify` with signature-verifying backend
   - new request format: pubkey + challenge + signature
   - verify challenge, then verify signature, then return fingerprint

### Phase 2 - Update claim and resolve endpoints - status: open

1. [ ] Update `resolveAccessRequest` and `claimMailboxRequest` structs
   - add `Challenge` and `Signature` fields
   - `edproof` field remains (now just the pubkey)
2. [ ] Update `handleClaimMailbox` and `handleResolveAccess`
   - pass challenge + signature through to verifier
3. [ ] Backward compatibility: if challenge/signature are empty, reject with clear error message
   - `{"error": "edproof now requires challenge-response — call POST /v1/auth/challenge first"}`

### Phase 3 - Tests - status: open

1. [ ] Unit tests for challenge generation and verification
   - happy path, expired challenge, tampered challenge, wrong pubkey
2. [ ] Unit tests for Ed25519 signature verification
   - valid signature, wrong key, wrong message, malformed inputs
3. [ ] Integration test: full challenge → sign → resolve flow
   - generate real Ed25519 keypair in test
   - request challenge, sign it, call resolve, verify access returned
4. [ ] Update existing edproof tests for new format

### Phase 4 - Config, wiring, deploy - status: open

1. [ ] Generate and set `EDPROOF_HMAC_SECRET` in GitHub secrets
2. [ ] Update deploy workflow to include the secret
3. [ ] Update `.env.example`
4. [ ] Update homepage/agent prompt text to reflect new flow

## Verification

- `go test ./...` passes
- Calling resolve with just a public key (no challenge/signature) returns a clear error
- Full flow works: challenge → sign → resolve returns IMAP credentials
- Tampered challenge or wrong private key → rejected

## Adjustments

## Progress Log
