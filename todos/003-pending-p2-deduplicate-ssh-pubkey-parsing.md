---
status: complete
priority: p2
issue_id: "003"
tags: [code-review, quality, edproof]
---

# Deduplicate SSH public key parsing across three functions

## Problem Statement

Three functions independently parse the `ssh-ed25519 <base64> <comment>` format:

1. `extractPubkeyBlob` (challenge.go:280-293) — splits fields, validates base64, returns blob string
2. `extractEd25519Key` (challenge.go:297-337) — splits fields, decodes base64, parses wire format
3. `FingerprintFromPubkey` (verifier.go:69-83) — splits fields, decodes base64, computes SHA256

Each re-validates the key type, splits on whitespace, and decodes base64. ~30 LOC of duplicated validation.

Additionally, `extractPubkeyBlob` decodes base64 solely to validate it (line 289), then discards the result and returns the string — a redundant allocation.

## Findings

- Flagged by: code-simplicity-reviewer (HIGH), architecture-strategist (MEDIUM), pattern-recognition (MEDIUM)
- Also: `makeTestSSHPubkey` duplicated across test files (challenge_test.go:18, handler_test.go:635)

## Proposed Solution

Extract a shared `parseSSHPubkey(pubkey string) (blob []byte, err error)` that validates format, decodes base64, and returns the raw blob. All three callers become thin wrappers:
- `extractPubkeyBlob` → `base64.StdEncoding.EncodeToString(blob)`
- `extractEd25519Key` → parse wire format from blob
- `FingerprintFromPubkey` → `sha256.Sum256(blob)`

Also: delete `writeTestSSHString` in challenge_test.go (duplicate of production `writeSSHString`).

- Effort: Small (1 hour)
- LOC reduction: ~30-40 lines

## Acceptance Criteria

- [ ] Single `parseSSHPubkey` function used by all three callers
- [ ] No redundant base64 decode-to-validate
- [ ] `writeTestSSHString` removed, tests use production `writeSSHString`
- [ ] All existing tests pass
