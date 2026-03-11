---
status: pending
priority: p2
issue_id: "002"
tags: [code-review, security, edproof]
---

# Enforce SSHSIG namespace to "edproof"

## Problem Statement

`sshsigNamespace = "edproof"` is defined at `challenge.go:132` but never checked. The SSHSIG verifier reads the namespace from the blob and passes it through without validation. A signature created for a different purpose (e.g., `namespace="file"` from `ssh-keygen -Y sign -n file`) could be replayed as an edproof authentication signature.

Practical risk is mitigated by the challenge needing to be a valid HMAC-authenticated string, but defense-in-depth calls for namespace enforcement.

## Findings

- `challenge.go:132` — `sshsigNamespace` constant defined but unused
- `challenge.go:178-183` — namespace read from blob, never compared
- `challenge_test.go:486` — `TestVerifySignatureSSHSIGAnyNamespace` confirms this is intentional
- Flagged by: security-sentinel (MEDIUM), architecture-strategist (MEDIUM), code-simplicity-reviewer

## Proposed Solution

Add `if string(namespace) != sshsigNamespace { return ErrSignatureInvalid }` after reading the namespace in `verifySSHSig`. Update the "any namespace" test to expect rejection, add a test for the correct namespace.

- Effort: Trivial (15 min)

## Acceptance Criteria

- [ ] SSHSIG signatures with namespace != "edproof" are rejected
- [ ] `sshsigNamespace` constant is actually used
- [ ] Test updated to verify namespace enforcement
