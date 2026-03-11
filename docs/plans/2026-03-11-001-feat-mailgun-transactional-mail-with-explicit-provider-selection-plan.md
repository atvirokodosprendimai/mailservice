---
title: "feat: Add Mailgun transactional mail with explicit provider selection"
type: feat
status: completed
date: 2026-03-11
---

# feat: Add Mailgun transactional mail with explicit provider selection

## Overview

Add Mailgun as a transactional mail provider implementing `ports.Notifier`, and replace the current silent notifier cascade (`selectNotifier()`) with explicit `NOTIFIER_PROVIDER` env var selection.
The cascade remains as a deprecated fallback for backward compatibility.

## Problem Statement / Motivation

- The service needs Mailgun as a sending option (reliable, well-known, good deliverability).
- The current `selectNotifier()` cascade (Unsend > Resend > SendGrid > Log) silently picks a provider based on which env vars happen to be set.
  This violates the lesson learned from the database mode incident: silent fallbacks hide misconfiguration.
- Explicit selection makes the deployment configuration deterministic and auditable.

## Proposed Solution

1. **New Mailgun adapter** (`internal/adapters/notify/mailgun_notifier.go`) using raw HTTP, consistent with Unsend/Resend pattern.
2. **Explicit provider selection** via `NOTIFIER_PROVIDER` env var validated in `config.Load()`.
3. **Deprecation path**: when `NOTIFIER_PROVIDER` is unset, fall back to existing cascade with a startup warning.

## Technical Considerations

### Mailgun API differences from existing adapters

| Aspect | Unsend / Resend | Mailgun |
|--------|----------------|---------|
| Auth | Bearer token | HTTP Basic Auth (`api` : API key) |
| Content-Type | `application/json` | `application/x-www-form-urlencoded` |
| Endpoint | Fixed URL | `POST /v3/{domain}/messages` (domain in path) |
| Region | N/A | US (`api.mailgun.net`) or EU (`api.eu.mailgun.net`) |

### Security considerations

- **Domain sanitization**: `MAILGUN_DOMAIN` is embedded in the URL path.
  Validate hostname format in constructor (alphanumeric, dots, hyphens only) to prevent path traversal.
- **API key in errors**: Basic Auth goes in the `Authorization` header, not the URL.
  Never log or return the API key in error messages.

### Performance implications

- Form-encoding HTML bodies is slightly larger than JSON but negligible for transactional mail.
- No retry/circuit-breaker (consistent with existing adapters).

## Acceptance Criteria

- [x] Mailgun adapter implements `ports.Notifier` (`SendPaymentLink`, `SendRecoveryLink`)
- [x] Raw HTTP with Basic Auth and form-urlencoded body
- [x] Configurable base URL (default: `https://api.mailgun.net`, EU: `https://api.eu.mailgun.net`)
- [x] `NOTIFIER_PROVIDER` env var selects notifier explicitly
- [x] Valid values: `unsend`, `resend`, `sendgrid`, `mailgun`, `log` (lowercase, case-sensitive)
- [x] When `NOTIFIER_PROVIDER` is set, required credentials for that provider are validated in `config.Load()` — missing credentials = startup error
- [x] When `NOTIFIER_PROVIDER` is unset, cascade runs with deprecation warning (logged once at startup)
- [x] Invalid `NOTIFIER_PROVIDER` value = startup error
- [x] httptest-based tests for Mailgun adapter (happy path, error responses, Basic Auth, form encoding, domain in URL, EU base URL)
- [x] Tests for `selectNotifier` with explicit `NOTIFIER_PROVIDER` values
- [x] `.env.example` updated with Mailgun vars and `NOTIFIER_PROVIDER`

## Environment Variables

```
# Explicit notifier selection (recommended)
# Valid: unsend, resend, sendgrid, mailgun, log
# If unset, falls back to legacy cascade (deprecated)
NOTIFIER_PROVIDER=mailgun

# Mailgun configuration (required when NOTIFIER_PROVIDER=mailgun)
MAILGUN_API_KEY=key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
MAILGUN_DOMAIN=mg.example.com
MAILGUN_FROM_EMAIL=noreply@example.com

# Optional
MAILGUN_FROM_NAME=MailService
MAILGUN_BASE_URL=https://api.mailgun.net   # default; use https://api.eu.mailgun.net for EU
```

## Implementation Phases

### Phase 1: Configuration and provider selection refactor

**Files to modify:**

- `internal/platform/config/config.go` — add `NotifierProvider`, `MailgunAPIKey`, `MailgunDomain`, `MailgunFromEmail`, `MailgunFromName`, `MailgunBaseURL` fields to `Config` struct and `Load()`.
  Add validation: when `NotifierProvider` is non-empty, validate it against allowed values and check required credentials for the selected provider.
- `cmd/app/main.go` — refactor `selectNotifier()` to branch on `cfg.NotifierProvider` first (switch statement), cascade as default case with deprecation log.

**Tests:**

- `cmd/app/notifier_test.go` — add test cases for explicit `NOTIFIER_PROVIDER` selection, invalid values, missing credentials.
  Existing cascade tests remain as regression tests for the deprecated path.

### Phase 2: Mailgun adapter

**Files to create:**

- `internal/adapters/notify/mailgun_notifier.go`

  ```go
  type MailgunNotifier struct {
      apiKey    string
      domain    string
      baseURL   string
      fromEmail string
      fromName  string
      client    *http.Client
  }

  func NewMailgunNotifier(apiKey, domain, baseURL, fromEmail, fromName string) (*MailgunNotifier, error)
  // Validates domain format, sets default baseURL if empty

  func (n *MailgunNotifier) SendPaymentLink(ctx context.Context, ownerEmail, paymentURL, mailboxID string) error
  func (n *MailgunNotifier) SendRecoveryLink(ctx context.Context, ownerEmail, recoveryURL string) error

  // private
  func (n *MailgunNotifier) send(ctx context.Context, to, subject, html string) error
  // POST {baseURL}/v3/{domain}/messages
  // Content-Type: application/x-www-form-urlencoded
  // Authorization: Basic base64("api:{apiKey}")
  // Body: from=Name <email>&to=recipient&subject=...&html=...
  ```

- `internal/adapters/notify/mailgun_notifier_test.go`

  Test cases:
  - `TestMailgunSendPaymentLink` — happy path, verify Basic Auth header, form-encoded body, correct URL path with domain
  - `TestMailgunSendRecoveryLink` — happy path
  - `TestMailgunNon2xxResponse` — verify error includes status code and response body (truncated)
  - `TestMailgunEUBaseURL` — verify EU endpoint is used when configured
  - `TestMailgunDomainValidation` — reject domains with path traversal characters
  - `TestMailgunFromFormat` — verify `"Name <email>"` format when fromName is set

### Phase 3: Wiring and documentation

**Files to modify:**

- `cmd/app/main.go` — add `case "mailgun":` to the refactored `selectNotifier()`, call `NewMailgunNotifier(...)`.
- `.env.example` — add `NOTIFIER_PROVIDER` and all `MAILGUN_*` vars with comments.

## Dependencies & Risks

- **Breaking change (mitigated)**: Existing deployments without `NOTIFIER_PROVIDER` will see a deprecation warning but continue working via the cascade.
  Future version removes the cascade entirely.
- **Mailgun account required for manual testing**: Unit tests use httptest, but end-to-end verification needs a Mailgun sandbox domain.
- **No new Go dependencies**: Raw HTTP + `net/url` for form encoding, `encoding/base64` for Basic Auth — all stdlib.

## Success Metrics

- Mailgun sends payment link and recovery link emails successfully (verified via Mailgun dashboard or sandbox).
- Existing notifiers continue to work unchanged.
- App refuses to start with invalid `NOTIFIER_PROVIDER` or missing credentials.

## Sources & References

- Existing adapter reference: `internal/adapters/notify/unsend_notifier.go`
- Ports definition: `internal/core/ports/ports.go:93-96`
- Current wiring: `cmd/app/main.go:131-142` (`selectNotifier()`)
- Anti-fallback learning: `memory/learning - 2603110114 - silent fallbacks hide failures.md`
- Unsend spec: `docs/specs/unsend-transactional-mail.md`
- Mailgun API docs: `https://documentation.mailgun.com/docs/mailgun/api-reference/openapi-final/tag/Messages/`
