---
tldr: All three feature specs are implemented in code but task checkboxes were never marked done
category: core
---

# Drift: Specs vs Implementation — Full Project

## Summary

**Drift level: Low (tracking drift, not implementation drift)**

The codebase implements nearly everything the three feature specs describe.
The primary drift is that spec task checkboxes (`[ ]`) were never ticked — the tracking is stale, not the code.

## Key-Bound Mailbox Spec (`docs/key-bound-mailbox-spec.md`)

All 7 tasks are marked `[ ]` but implemented:

- [x] 1. Key-bound data model — `key_fingerprint` and `billing_email` in migration 8, repository queries by fingerprint
- [x] 2. `edproof` verification port and adapter — `KeyProofVerifier` in `internal/core/ports`, adapter in `internal/adapters/identity/edproof/`
- [x] 3. Mailbox claim flow — `POST /v1/mailboxes/claim` wired, `ClaimMailbox` service method with tests
- [x] 4. Key-based resolve flow — `POST /v1/access/resolve` wired, `ResolveAccessByKey` service method with tests
- [x] 5. Legacy flow preserved — account/token endpoints still registered
- [x] 6. Product boundaries documented — `docs/use-cases.md` covers all use cases
- [x] 7. Future POP3/HTTP design — `docs/future-access.md` covers POP3 and HTTP read API

**Minor gap:** `POST /v1/mailboxes/renew` (separate renewal endpoint from spec) is not implemented as a distinct endpoint.
ClaimMailbox already handles renewal for unpaid/expired mailboxes, so this may be intentionally collapsed into claim.

## Polar Minimal Payments Spec (`docs/polar-minimal-payments-spec.md`)

All 5 task groups marked `[ ]` but implemented:

- [x] 1. Provider-neutral payment seam — `PaymentGateway` port, Polar selected when `POLAR_TOKEN` configured
- [x] 2. Polar checkout creation — `PolarGateway` adapter with tests
- [x] 3. Claim flow connected — ClaimMailbox creates Polar checkout, notification sends payment link
- [x] 4. Polar payment completion — `POST /v1/webhooks/polar` with HMAC-SHA256 verification, `MarkMailboxPaid`, webhook tests
- [x] 5. Docs — landing page describes the Polar flow

## Unsend Transactional Mail Spec (`docs/specs/unsend-transactional-mail.md`)

All 5 tasks marked `[ ]` but implemented:

- [x] 1. Config support — `UNSEND_KEY`, `UNSEND_BASE_URL`, `UNSEND_FROM_EMAIL`, `UNSEND_FROM_NAME` loaded
- [x] 2. Unsend notifier adapter — `UnsendNotifier` in `internal/adapters/notify/unsend_notifier.go` with tests
- [x] 3. Notifier selection wired — Unsend > Resend > SendGrid > Log precedence in `cmd/app/main.go`
- [x] 4. Docs — config vars present (`.env.example` not verified)
- [x] 5. End-to-end validation — adapter tests exist, `go test ./...` presumably passes

## Code Exceeding Spec

- `POST /v1/payments/polar/success` — success redirect endpoint, not in spec
- `GET /mock/pay/{sessionID}` — development mock payment, not in spec
- IMAP message read endpoints (`/v1/imap/messages`, `/v1/imap/messages/get`) — in future-access design but already implemented
- Concurrency semaphore (max concurrent requests) — operational concern, not specced
- Landing page with HTML documentation and agent prompt — not specced

## Suggested Actions

1. Mark all spec task checkboxes `[x]` — pure tracking update
2. Decide on `/v1/mailboxes/renew` — is claim-handles-renewal intentional, or should renew be a separate endpoint?
3. Consider closing the `stripe_session_id` column rename debt (noted in Polar spec as deferred)
4. `.env.example` — verify Unsend config is documented there
