---
title: "Fix P2 security findings: timing-safe auth, body size limits, HTML escaping"
category: security-issues
date: 2026-03-11
tags: [timing-attack, memory-exhaustion, xss, go, mailservice, code-review, admin-api, mailgun, input-validation]
severity: P2
components: [internal/adapters/httpapi/handler.go, internal/adapters/notify/mailgun_notifier.go]
---

# P2 Security Hardening: Constant-Time Auth, Body Size Limits, HTML Escaping

## Problem

Code review of the mailservice (Go) surfaced three P2 security findings:

1. **Timing attack on admin key comparison** — `handler.go` used `==` to compare the admin API key, leaking information about correct bytes via response timing.
2. **Unbounded request body reads** — `json.NewDecoder(r.Body)` and `io.ReadAll(r.Body)` had no size limit, allowing memory exhaustion via large payloads.
3. **XSS in email templates** — `mailgun_notifier.go` interpolated user-controlled values (`mailboxID`, `paymentURL`, `recoveryURL`) directly into HTML email bodies without escaping.

## Root Cause

All three are instances of missing defensive coding at system boundaries: secret comparison without constant-time guarantees, input reading without size bounds, and output rendering without escaping.

## Solution

### Fix 1: Constant-Time Admin Key Comparison

Replace `==` with `crypto/subtle.ConstantTimeCompare`, which always compares every byte regardless of where a mismatch occurs.

```go
import "crypto/subtle"

// BEFORE (vulnerable):
if token == h.adminAPIKey { ... }

// AFTER (constant-time):
if subtle.ConstantTimeCompare([]byte(token), []byte(h.adminAPIKey)) != 1 {
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}
```

`ConstantTimeCompare` returns `1` for equal inputs and `0` otherwise.
When inputs differ in length it still runs in constant time and returns `0`.

### Fix 2: Request Body Size Limits

Define a single constant and wrap every `r.Body` read with `io.LimitReader`.

```go
const maxRequestBodyBytes = 1 << 20 // 1 MB

// JSON decoding (decodeJSON helper):
json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(&target)

// Webhook raw body reads:
io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
```

Payloads exceeding 1 MB are truncated at `io.EOF`, causing the JSON decoder to fail with a parse error or webhook signature verification to fail — both safe failure modes.

### Fix 3: HTML Escaping in Email Templates

Escape all user-controlled values with `html.EscapeString()` before interpolation.
Rename the local variable `html` to `body` to avoid shadowing the `html` package import.

```go
import "html"

// BEFORE (vulnerable, 'html' variable shadows the package):
html := fmt.Sprintf(`<p>Mailbox <strong>%s</strong>...</p>`, mailboxID)

// AFTER (escaped, variable renamed):
body := fmt.Sprintf(
    `<p>Mailbox <strong>%s</strong>...</p>`,
    html.EscapeString(mailboxID),
)
```

Applied to `mailboxID`, `paymentURL`, and `recoveryURL` across `SendPaymentLink` and `SendRecoveryLink`.

## Prevention

| Issue | Rule | Static analysis |
|-------|------|-----------------|
| Timing attack | Never use `==` for secret comparison; use `subtle.ConstantTimeCompare` | Semgrep custom rule on `$SECRET == $INPUT` |
| Unbounded reads | Always wrap `r.Body` with `io.LimitReader` or `http.MaxBytesReader` before reading | Semgrep: flag bare `json.NewDecoder($REQ.Body)` |
| XSS in templates | Always escape user-controlled values in HTML output | `gosec` G203 + Semgrep for `text/template` or `fmt.Sprintf` producing HTML |

**Code review checklist items:**
- Any `==` against a variable named `key`, `token`, `secret`, `apiKey` → flag for constant-time comparison.
- Any `json.NewDecoder(r.Body)` or `io.ReadAll(r.Body)` without a preceding size limit → flag.
- Any `fmt.Sprintf` producing HTML with user data → flag; prefer `html/template` or explicit `html.EscapeString`.

## Related Documentation

- [Ed25519 challenge-response auth](../security-issues/ed25519-challenge-response-auth.md) — the implementation whose code review surfaced these findings
- `todos/001-pending-p1-hmac-secret-validation.md` — related HMAC secret hardening
- `todos/005-pending-p2-signature-size-guard.md` — analogous input size guard for signatures
- `todos/006-pending-p3-minor-cleanups.md` — originally tracked the constant-time comparison item

## Verification

All existing tests pass after fixes.
Deployed to production successfully with health check green.
