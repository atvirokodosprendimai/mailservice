---
title: "feat: Add EdProof-authenticated support contact endpoint"
type: feat
status: active
date: 2026-03-17
---

# feat: Add EdProof-authenticated support contact endpoint

## Overview

Add `POST /v1/support/messages` so agents can contact support via HTTP.
Agents cannot send email (inbound-only service), but they can authenticate via EdProof and POST.
The service forwards the message to the support mailbox via the existing Notifier (SendGrid/Mailgun).

Support mailbox: `mbx_014d51a9d0b@truevipaccess.com` (configurable via env).

## Problem Statement / Motivation

Agents are the primary users of the service.
They have no way to report issues, ask questions, or request help — they can't send email.
The only contact path today is `hi@truevipaccess.com` in the HTML footer, which requires a human operator.
An HTTP-native support channel lets agents self-serve and dogfoods the product.

## Proposed Solution

### Flow

```
Agent                          Service                         Support Mailbox
  |                               |                                |
  |-- POST /v1/auth/challenge --> |                                |
  |<-- challenge (30s TTL) -------|                                |
  |                               |                                |
  |-- POST /v1/support/messages ->|                                |
  |   {edproof, challenge,        |                                |
  |    signature, subject, body}  |                                |
  |                               |-- verify EdProof               |
  |                               |-- lookup mailbox by fingerprint|
  |                               |-- check rate limit (3/hr)      |
  |                               |-- persist to support_messages  |
  |                               |-- Notifier.SendSupportMessage -|-> email arrives
  |<-- {"status":"sent"} ---------|                                |
```

### Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Auth | EdProof (challenge-response) | Matches all existing agent endpoints |
| Mailbox status check | None — any status allowed | Expired/unpaid agents need support most |
| Content format | Plain text only, HTML-escaped before sending | Prevents XSS/injection in support mailbox |
| Rate limit | 3 messages/hour per key fingerprint | Prevents abuse without blocking legitimate use |
| Rate limit storage | `support_messages` table (count recent rows) | Doubles as audit trail, survives restarts |
| Persistence | Yes — `support_messages` table | Audit trail + rate limiting |
| Idempotency | None | Duplicate support messages are acceptable |
| Reply-To | Agent's mailbox address | Support team can reply directly to the agent's inbox |
| Support address | Env var `SUPPORT_EMAIL`, default `mbx_014d51a9d0b@truevipaccess.com` | Config hygiene |
| Response | `{"status":"sent"}` HTTP 200 | Simple, matches existing patterns |
| Service owner | `MailboxService` | Already has notifier + mailbox repo dependencies |

## Technical Considerations

### Architecture

- **New endpoint:** `POST /v1/support/messages` in `handler.go` Routes()
- **New Notifier method:** `SendSupportMessage(ctx, fromMailboxID, fromEmail, replyTo, subject, body) error`
  - All 5 notifier implementations must be updated (SendGrid, Mailgun, Resend, Unsend, Log)
- **New table:** `support_messages` (migration)
- **New service method:** `MailboxService.SendSupportMessage(...)`
- **Update:** `/docs/agent-api-skill.md` so agents discover the endpoint

### Request/Response

```go
// Request
type supportMessageRequest struct {
    EDProof   string `json:"edproof"`
    Challenge string `json:"challenge"`
    Signature string `json:"signature"`
    Subject   string `json:"subject"`
    Body      string `json:"body"`
}

// Response (200 OK)
{"status": "sent"}

// Errors
// 400 — missing/invalid fields, subject or body empty, body too long
// 401 — EdProof verification failed (expired, tampered, invalid signature)
// 404 — no mailbox found for key fingerprint
// 429 — rate limit exceeded (3/hour)
// 500 — notifier failure
```

### Validation Rules

- `subject`: required, max 200 chars, plain text, trimmed
- `body`: required, max 10,000 chars, plain text, trimmed
- Both HTML-escaped before insertion into email template

### Email Format (received by support team)

```
From: noreply@truevipaccess.com
To: mbx_014d51a9d0b@truevipaccess.com
Reply-To: mbx_abc123@truevipaccess.com
Subject: [mbx_abc123] Agent's subject line

--- Agent Support Message ---
Mailbox:     mbx_abc123
Fingerprint: SHA256:xYz...
Status:      active
Owner:       owner@example.com
---

Agent's message body here.
```

### Database Migration

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS support_messages (
    id TEXT PRIMARY KEY,
    mailbox_id TEXT NOT NULL,
    key_fingerprint TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_support_messages_fingerprint_created
    ON support_messages(key_fingerprint, created_at);

-- +goose Down
DROP TABLE IF EXISTS support_messages;
```

### Security

- All agent-provided content (`subject`, `body`) is `html.EscapeString()`'d before email template insertion
- No header injection risk — SendGrid/Mailgun use structured APIs, not raw SMTP
- Rate limiting prevents mailbox flooding and email quota exhaustion
- Challenge reuse within 30s TTL is acceptable — rate limit is per-fingerprint-per-hour, not per-challenge

## System-Wide Impact

- **Notifier interface change**: All implementations must add `SendSupportMessage`. This is a breaking interface change but all implementations are internal.
- **No effect on existing endpoints**: New endpoint is independent.
- **Email quota**: 3/hr/agent is low — even 1000 agents = 3000 emails/hr max, well within SendGrid/Mailgun limits.
- **Landing page**: Update footer contact section to mention API endpoint for agents.

## Acceptance Criteria

- [ ] `POST /v1/support/messages` accepts EdProof-authenticated requests
- [ ] Agents with any mailbox status (active, pending_payment, expired) can send messages
- [ ] Messages are persisted to `support_messages` table
- [ ] Messages are forwarded to configurable support email via Notifier
- [ ] Email includes sender mailbox ID, fingerprint, status, owner email, and Reply-To header
- [ ] Subject max 200 chars, body max 10,000 chars, both required
- [ ] Rate limited to 3 messages/hour per key fingerprint
- [ ] All agent-provided content is HTML-escaped
- [ ] Agent API skill doc (`/docs/agent-api-skill.md`) updated with new endpoint
- [ ] Landing page footer updated to mention HTTP support endpoint for agents
- [ ] Tests cover: happy path, auth failures, rate limiting, mailbox-not-found, notifier failure, validation errors

## Success Metrics

- Support messages arrive in the support mailbox with correct metadata
- Reply-To allows support team to respond directly to agent inbox
- Rate limiting blocks abuse without blocking legitimate use

## Dependencies & Risks

- **Notifier interface change**: Low risk — all implementations are internal, no external consumers
- **Email deliverability**: Support messages must not trigger spam filters. Using the existing transactional sender and structured subject helps.
- **Challenge reuse**: An agent could send 3 messages with one challenge within 30s. The hourly rate limit handles this.

## Implementation Phases

### Phase 1: Database + Domain

- [ ] Add `support_messages` migration
- [ ] Add domain struct if needed (or use inline map)

### Phase 2: Notifier Extension

- [ ] Add `SendSupportMessage` to `Notifier` interface in `ports.go`
- [ ] Implement in SendGrid notifier (HTML-escaped plain text template)
- [ ] Implement in Mailgun notifier
- [ ] Implement in Resend notifier
- [ ] Implement in Unsend notifier
- [ ] Implement in Log notifier
- [ ] Add `SUPPORT_EMAIL` to config

### Phase 3: Service + Handler

- [ ] Add `SendSupportMessage` to `MailboxService`
  - Verify EdProof
  - Lookup mailbox by fingerprint (any status)
  - Check rate limit (count recent rows in `support_messages`)
  - Persist message
  - Forward via notifier
- [ ] Add `POST /v1/support/messages` handler
- [ ] Add route to `Routes()`
- [ ] Validate request fields (subject, body length/required)

### Phase 4: Documentation + Landing Page

- [ ] Update `/docs/agent-api-skill.md` with new endpoint
- [ ] Update landing page footer/instructions

### Phase 5: Tests

- [ ] Handler tests: happy path, auth failures, validation, rate limit, notifier error
- [ ] Service tests: rate limit logic, message persistence
- [ ] Notifier tests: email formatting, HTML escaping

## Sources & References

- EdProof auth: `internal/adapters/identity/edproof/challenge.go`
- Handler pattern: `internal/adapters/httpapi/handler.go:692` (handleClaimMailbox — same EdProof pattern)
- Notifier interface: `internal/core/ports/ports.go:111-114`
- Migration pattern: `internal/platform/database/migrations/`
- Agent API skill: `docs/agent-api-skill.md` (embedded via `docs/embed.go`)
- Related todo: `memory/todo - 2603170902 - add support contact channel via mailservice mailbox.md`
