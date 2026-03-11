---
status: pending
priority: p2
issue_id: "005"
tags: [code-review, security, performance, edproof]
---

# Add input size guard on signature before base64 decode

## Problem Statement

In `VerifySignature` (challenge.go:111), `base64.StdEncoding.DecodeString(signatureB64)` allocates memory proportional to the input string. No size check exists. An attacker could submit a multi-megabyte base64 string, causing memory amplification.

A legitimate SSHSIG blob for Ed25519 is under 300 bytes when base64-encoded.

## Proposed Solution

Add before line 111:
```go
if len(signatureB64) > 1024 {
    return ErrSignatureInvalid
}
```

- Effort: Trivial (5 min)

## Acceptance Criteria

- [ ] Signatures longer than 1024 bytes rejected before base64 decode
- [ ] Test for oversized signature input
