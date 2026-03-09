# Polar Minimal Payments Spec

Ticket: `mai-bpn7`

## Goal

Deliver a minimal production-ready payment slice for the key-bound mailbox flow:

1. agent claims a mailbox with `billing_email` and `edproof`
2. service creates a Polar checkout
3. billing contact receives or is shown the payment link
4. Polar payment completion activates the mailbox for one month
5. the same key can resolve IMAP access after payment

This spec is intentionally narrow. It does not remove legacy Stripe/mock flows in the same change.

## Requirements

### User Stories

- As an agent, I want a claimed mailbox to return a real Polar payment link, so that I can activate it today.
- As a billing contact, I want the payment link sent to the billing email, so that anyone with access to that inbox can pay it.
- As the service, I want Polar payment completion to activate the mailbox bound to the key, so that IMAP access works after payment.
- As an operator, I want the new payment path to coexist with the legacy flow during migration, so that existing clients keep working.

### Acceptance Criteria

1. WHEN `POST /v1/mailboxes/claim` creates a new key-bound mailbox THEN the system SHALL create a Polar checkout session and persist its provider session identifier and payment URL.
2. WHEN `POST /v1/mailboxes/claim` is called for an existing unpaid or expired key-bound mailbox THEN the system SHALL create a fresh Polar checkout session and update the stored payment session fields.
3. WHEN `POST /v1/mailboxes/claim` is called for an already active key-bound mailbox THEN the system SHALL return the existing mailbox without creating a new checkout session.
4. WHEN a Polar checkout is created THEN the system SHALL associate it with the mailbox billing email and mailbox identifier.
5. WHEN a valid Polar payment completion callback is received THEN the system SHALL mark the matching mailbox paid and extend its subscription by one month.
6. IF a Polar callback cannot be matched to a mailbox payment session THEN the system SHALL reject it without activating any mailbox.
7. IF a Polar callback is invalid or unauthenticated THEN the system SHALL reject it without changing mailbox state.
8. WHEN legacy Stripe or mock payment flows are used by existing paths THEN the system SHALL continue to preserve their current behavior during this slice.

## Design

### Overview

The current key-bound claim flow already asks the payment gateway for a payment link and persists `PaymentSessionID` and `PaymentURL`. The minimal change is to make Polar the real provider for the new flow while preserving the provider-neutral seam in core.

The core service should continue to depend only on `ports.PaymentGateway`. Polar-specific request building, auth headers, and callback verification stay in adapters.

### Architecture

- Core service:
  - continues calling `PaymentGateway.CreatePaymentLink`
  - continues calling `MarkMailboxPaid(paymentSessionID)`
- Payment adapter:
  - add Polar gateway adapter under `internal/adapters/payment`
  - map mailbox/payment request fields into Polar checkout creation
- HTTP adapter:
  - add a Polar webhook or callback endpoint
  - validate the callback and extract the provider session identifier
  - call `MailboxService.MarkMailboxPaid`
- Config:
  - add Polar-specific config values needed for checkout creation and callback validation
  - use `POLAR_TOKEN` as the primary secret

### Components and Interfaces

#### Payment gateway port

Keep the current provider-neutral seam:

```go
type PaymentGateway interface {
    CreatePaymentLink(ctx context.Context, req PaymentLinkRequest) (*PaymentLink, error)
}
```

No Polar types should leak into `internal/core`.

#### Polar gateway adapter

Add a new adapter, likely:

- `internal/adapters/payment/polar_gateway.go`

Responsibilities:

- call Polar checkout/session creation API
- set product/price metadata for mailbox activation
- include mailbox id and billing email in metadata if the provider supports it
- return:
  - `SessionID`
  - `URL`

If Polar requires a product or checkout model different from Stripe, adapt internally. The core should still only see `PaymentLink`.

#### Callback handling

Add a minimal HTTP path for Polar payment completion, for example:

- `POST /v1/webhooks/polar`

Responsibilities:

- verify callback authenticity using configured Polar secret or token-based scheme
- ignore unsupported event types
- extract the provider session id for successful payment events
- call `MailboxService.MarkMailboxPaid`

This slice should support only the success path needed to activate mailboxes. Broader event processing can come later.

### Data Model

No schema redesign is required for this slice.

Use existing mailbox fields:

- `payment_session_id`
- `payment_url`
- `billing_email`
- `status`
- `paid_at`
- `expires_at`

Note:
- the physical SQLite column is still `stripe_session_id`
- the Go/domain name is already provider-neutral: `PaymentSessionID`

This is acceptable for today. A DB column rename is not required for the minimal slice.

### Config

Minimum expected config additions or usage:

- `POLAR_TOKEN`
- `POLAR_WEBHOOK_SECRET` if Polar webhook verification requires a separate secret
- success/cancel URLs if required by Polar checkout creation

The app bootstrap in `cmd/app/main.go` should prefer Polar for the new path when `POLAR_TOKEN` is configured. Legacy provider wiring can stay available during migration.

### Error Handling

- If Polar checkout creation fails, `POST /v1/mailboxes/claim` should return an internal error and not create a mailbox with empty payment fields.
- If notifier delivery fails after checkout creation, return an error as the service does today.
- If the Polar callback is invalid, return `400` or `401` and do not mutate mailbox state.
- If the callback is valid but the session id is unknown, return `404` or `400` and do not mutate mailbox state.
- If the callback repeats an already-applied payment event, handle it idempotently by reusing `MarkMailboxPaid` semantics and avoiding broken state.

### Decisions

#### Decision: Keep the core payment seam provider-neutral

Context:
The minimal Polar slice should not reintroduce provider-specific logic into the service layer.

Options considered:
1. Add Polar-specific methods to core
2. Reuse the existing `PaymentGateway` abstraction

Decision:
Reuse the existing `PaymentGateway` abstraction.

Rationale:
The current seam is already sufficient for checkout creation. It keeps today’s change small and preserves room for future provider swaps.

#### Decision: Do not rename the database column in this slice

Context:
The mailbox table still stores provider session ids in `stripe_session_id`.

Options considered:
1. Rename the DB column now
2. Keep the DB column and use provider-neutral Go names

Decision:
Keep the DB column for now.

Rationale:
The column rename adds migration risk without product benefit today. The Go/domain layer is already neutral enough for the minimal Polar rollout.

#### Decision: Keep legacy flows alive during the slice

Context:
The repo still contains Stripe and mock code paths used by legacy behavior.

Options considered:
1. Remove Stripe/mock immediately
2. Add Polar for the key-bound flow and defer legacy removal

Decision:
Add Polar for the key-bound flow and defer legacy removal.

Rationale:
This gives immediate value today without forcing a broad migration in one change.

## Testing Strategy

- Unit tests for Polar gateway request/response mapping
- Service tests for claim flow behavior:
  - new mailbox creates checkout
  - unpaid mailbox refreshes checkout
  - active mailbox does not create checkout
- Handler tests for Polar callback:
  - valid completion activates mailbox
  - invalid signature/token is rejected
  - unknown session id is rejected
- Regression test ensuring legacy Stripe/mock behavior still compiles and existing tests pass

## Tasks

- [x] 1. Add or tighten the provider-neutral payment seam
- [x] 1.1 Confirm `PaymentGateway` and payment DTOs are sufficient for Polar checkout creation
  - adjust naming only if needed without leaking provider types into core
  - _Requirements: 1, 2, 8_
- [x] 1.2 Wire app bootstrap to select Polar when `POLAR_TOKEN` is configured for the new flow
  - keep legacy provider wiring available during migration
  - _Requirements: 8_

- [x] 2. Implement Polar checkout creation
- [x] 2.1 Add Polar payment adapter under `internal/adapters/payment`
  - create checkout/session and map response to `ports.PaymentLink`
  - _Requirements: 1, 2, 4_
- [x] 2.2 Cover successful and failing checkout creation with adapter tests
  - _Requirements: 1, 2_

- [x] 3. Connect claim flow to Polar checkout behavior
- [x] 3.1 Ensure key-bound claim creates or refreshes payment session data correctly
  - _Requirements: 1, 2, 3_
- [x] 3.2 Ensure payment link notification/response works with Polar URLs
  - _Requirements: 4_

- [x] 4. Handle Polar payment completion
- [x] 4.1 Add minimal Polar webhook/callback endpoint
  - verify callback authenticity and extract session id
  - _Requirements: 5, 6, 7_
- [x] 4.2 Activate mailbox through `MarkMailboxPaid`
  - _Requirements: 5_
- [x] 4.3 Add invalid and duplicate callback tests
  - _Requirements: 6, 7_

- [x] 5. Update docs for the preferred payment path
- [x] 5.1 Update README and/or follow-up docs to describe Polar as the preferred key-bound payment path
  - _Requirements: 8_
