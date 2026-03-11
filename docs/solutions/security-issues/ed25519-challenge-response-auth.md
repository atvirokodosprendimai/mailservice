---
title: "Ed25519 Challenge-Response Implementation and SSHSIG Format Support"
category: "security-issues"
date: "2026-03-11"
tags: ["ed25519", "challenge-response", "hmac", "sshsig", "signature-verification", "stateless-auth"]
severity: "high"
component: "internal/adapters/identity/edproof"
symptoms:
  - "Public key accepted without verifying private key ownership"
  - "SSHSIG wrapped signatures (from ssh-keygen -Y sign) rejected as invalid"
  - "Base64-encoded signatures with newlines caused parsing failures"
root_cause:
  - "Original edproof implementation lacked challenge-response mechanism"
  - "VerifySignature only handled raw Ed25519 format, not SSHSIG wire format"
files_modified:
  - "internal/adapters/identity/edproof/challenge.go"
  - "internal/adapters/identity/edproof/challenge_test.go"
  - "internal/adapters/httpapi/handler.go"
  - "internal/platform/config/config.go"
  - "cmd/app/main.go"
  - "flake.nix"
---

# Ed25519 Challenge-Response Authentication and SSHSIG Format Support

## Problem

The edproof identity verification accepted any Ed25519 public key without requiring the client to prove ownership of the corresponding private key.
A client could submit any valid `ssh-ed25519` public key string — including one belonging to another user — and pass verification.
The key-based access control on mailbox claiming and IMAP access resolution provided no actual authentication guarantee.

After deploying the challenge-response fix, clients using `ssh-keygen -Y sign` got "signature verification failed" because the server only accepted raw 64-byte Ed25519 signatures, not the SSHSIG binary envelope format that ssh-keygen produces.

## Root Cause

The original verification path called `keyProofVerifier.Verify(ctx, pubkey)` directly — it validated that the key was syntactically valid and existed in the system, but never challenged the caller to sign anything.
There was no proof-of-possession step.

The SSHSIG failure was a format mismatch: `ssh-keygen -Y sign` produces a structured binary blob (magic + namespace + hash algorithm + SHA-512 of message + wrapped signature), not a raw Ed25519 signature over the message bytes.

## Investigation Steps

1. Traced `handleClaimMailbox` and `handleResolveAccess` — both delegated to `keyProofVerifier.Verify` with no signature check.
2. Confirmed no challenge endpoint existed (`/v1/auth/challenge` was absent).
3. Chose stateless HMAC approach to avoid requiring a nonce store (database or cache dependency).
4. After deployment, inspected the SSHSIG wire format spec when `ssh-keygen` signatures were rejected. The `U1NIU0lH` base64 prefix decodes to the "SSHSIG" magic.

## Solution

### Stateless Challenge-Response Protocol

Challenge format: `v1.<unix_ts>.<hex_nonce>.<hmac_hex>`

- HMAC input: `v1.<ts>.<nonce>.<canonical_pubkey_blob>` — binds the challenge to a specific key
- 30-second TTL, 5-second clock skew tolerance
- No server-side storage required

### Handler Dual-Mode Verification

```go
func (h *Handler) verifyEdproof(ctx context.Context, pubkey, challenge, signature string) (*ports.VerifiedKey, error) {
    if len(h.edproofHMACSecret) > 0 {
        if challenge == "" || signature == "" {
            return nil, errChallengeRequired
        }
        if err := edproof.VerifyChallenge(challenge, pubkey, h.edproofHMACSecret, 30*time.Second, h.now()); err != nil {
            return nil, err
        }
        if err := edproof.VerifySignature(challenge, pubkey, signature); err != nil {
            return nil, err
        }
    }
    return h.keyProofVerifier.Verify(ctx, pubkey)
}
```

When `EDPROOF_HMAC_SECRET` is unset, the service operates in legacy passthrough mode — no flag day for clients.

### Dual Signature Format Support

`VerifySignature` detects format by the `"SSHSIG"` magic prefix:

```go
if bytes.HasPrefix(sig, []byte(sshsigMagic)) {
    return verifySSHSig(rawKey, []byte(challenge), sig)
}
// Raw Ed25519: verify 64-byte signature directly
```

### SSHSIG Wire Format Parser

SSHSIG blob layout:

| Field | Type | Notes |
|---|---|---|
| Magic | 6 bytes | `"SSHSIG"` |
| Version | uint32 | must be 1 |
| Public key | SSH string | skipped (trusted from request) |
| Namespace | SSH string | e.g. `"edproof"` |
| Reserved | SSH string | empty |
| Hash algorithm | SSH string | `"sha512"` or `"sha256"` |
| Signature blob | SSH string | nested: key_type + raw_sig |

The signed data is reconstructed as:

```go
func buildSSHSigSignedData(namespace, hashAlgo string, messageHash []byte) []byte {
    var buf bytes.Buffer
    buf.WriteString("SSHSIG")
    writeSSHString(&buf, []byte(namespace))
    writeSSHString(&buf, nil)               // reserved
    writeSSHString(&buf, []byte(hashAlgo))
    writeSSHString(&buf, messageHash)       // SHA-512(challenge_string)
    return buf.Bytes()
}
```

The Ed25519 verification runs against this reconstructed envelope, not the raw challenge bytes.

## Prevention & Gotchas

### Never accept public keys without proof-of-possession

Any identity system based on public keys must require the caller to prove they hold the private key.
A public key alone is not a credential — it's an identifier.

### Always support SSHSIG when accepting SSH signatures

`ssh-keygen -Y sign` is the standard tool users reach for.
It produces SSHSIG format, not raw Ed25519.
Support both formats by checking for the magic prefix.

### Strip newlines from base64-encoded signatures

`ssh-keygen` outputs multi-line base64.
Clients must `tr -d '\n'` before sending.
Go's `base64.StdEncoding.DecodeString` is strict about whitespace.

### Update Nix vendorHash when adding Go dependencies

Adding `golang.org/x/crypto/ssh` changed the vendor directory hash.
The Nix build fails with `hash mismatch in fixed-output derivation` — update `vendorHash` in `flake.nix` with the hash from the error output.

## Testing

22+ tests in `challenge_test.go` covering:
- Challenge generation and verification (happy path, expired, tampered, wrong pubkey, future timestamp)
- Raw Ed25519 signature verification
- SSHSIG signature verification (with real ssh-keygen-compatible test data)
- Full integration flow (generate challenge, sign, verify)
- Unique nonce generation
- Malformed input handling

7 handler tests in `handler_test.go`:
- Challenge endpoint (valid key, invalid key, not configured)
- Full claim and resolve flows with challenge-response
- Missing challenge rejection
- Wrong signature rejection

## Related Documentation

- [Implementation plan](../../docs/plans/2026-03-11-002-feat-edproof-challenge-response-plan.md)
- [Key-bound mailbox spec](../../docs/key-bound-mailbox-spec.md)
