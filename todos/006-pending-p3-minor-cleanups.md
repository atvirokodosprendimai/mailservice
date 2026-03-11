---
status: pending
priority: p3
issue_id: "006"
tags: [code-review, quality, edproof]
---

# Minor code cleanups from review

## Items

### 1. `writeSSHString` stack allocation
`challenge.go:273` — `make([]byte, 4)` allocates on heap. Use `var lenBuf [4]byte` instead.

### 2. Collapse `writeEdproofError` switch
`handler.go:815-832` — four identical case arms can be collapsed into one with comma-separated `errors.Is` checks. Saves 8 LOC.

### 3. Default error status 503 vs 500
`handler.go:830` — `writeEdproofError` default case returns 503 (Service Unavailable). Codebase standard for unknown errors is 500 (Internal Server Error).

### 4. Admin API key constant-time comparison (pre-existing)
`handler.go:1218` — `token != h.adminAPIKey` uses `!=` which is not constant-time. Use `subtle.ConstantTimeCompare`. This is pre-existing, not from the edproof work.

### 5. Startup log for edproof mode
`main.go` — no log indicating whether challenge-response or passthrough mode is active. Other optional features (Polar, Stripe) log their configuration status.

## Effort

Small (30 min total for all items)
