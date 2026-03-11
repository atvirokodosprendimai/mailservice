---
status: complete
priority: p1
issue_id: "001"
tags: [code-review, security, edproof]
---

# Enforce HMAC secret minimum length and explicit passthrough opt-in

## Problem Statement

Two related issues:

1. **No minimum length on HMAC secret.** An operator can set `EDPROOF_HMAC_SECRET=a` and the system accepts it. HMAC-SHA256 needs at least 32 bytes of entropy for full security guarantees. A short secret enables brute-force attacks against challenge HMACs, allowing attackers to forge challenges for arbitrary public keys.

2. **Silent passthrough when secret is empty.** When `EDPROOF_HMAC_SECRET` is unset, `verifyEdproof` silently skips all challenge-response verification. The challenge endpoint returns 503, but claim/resolve endpoints accept any public key without proof of private key ownership. An operator who forgets the secret unknowingly runs without authentication.

## Findings

- `config.go:128` — raw `os.Getenv` with no length validation
- `handler.go:797` — `if len(h.edproofHMACSecret) > 0` silently falls through
- `handler.go:610` — challenge endpoint correctly returns 503, but claim/resolve don't
- Flagged by: security-sentinel (HIGH), architecture-strategist (HIGH)

## Proposed Solutions

### Option A: Startup validation (recommended)
- Validate secret is >= 32 bytes at startup in `config.go`
- Require explicit `EDPROOF_MODE=passthrough` env var to opt into insecure mode
- Log CRITICAL warning if passthrough mode is active
- Pros: Prevents misconfiguration, operator must consciously choose
- Cons: Breaking change for deployments without the secret
- Effort: Small

### Option B: Startup warning only
- Log a warning when secret is empty but don't block startup
- Keep existing passthrough behavior
- Pros: Non-breaking
- Cons: Warnings get ignored
- Effort: Trivial

## Acceptance Criteria

- [ ] `EDPROOF_HMAC_SECRET` shorter than 32 bytes rejected at startup (or requires explicit passthrough opt-in)
- [ ] Startup log clearly indicates whether challenge-response or passthrough mode is active
- [ ] Tests cover both valid and too-short secrets
