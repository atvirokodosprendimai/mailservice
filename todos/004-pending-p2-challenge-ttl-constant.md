---
status: pending
priority: p2
issue_id: "004"
tags: [code-review, quality, edproof]
---

# Extract challenge TTL into shared constant

## Problem Statement

The 30-second challenge TTL is hardcoded in two places:
- `handler.go:629` — `"expires_in": 30` in the challenge response
- `handler.go:802` — `30 * time.Second` in the verification call

If someone changes one without the other, the system silently breaks (client told one TTL, server enforces another).

## Proposed Solution

Define `const challengeMaxAge = 30 * time.Second` in handler.go (or in the edproof package). Use `int(challengeMaxAge.Seconds())` for the response and `challengeMaxAge` for verification.

- Effort: Trivial (10 min)

## Acceptance Criteria

- [ ] Single constant for challenge TTL
- [ ] Both response and verification reference same value
