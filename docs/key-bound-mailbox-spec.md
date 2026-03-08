# Key-Bound Inbound Mailbox Spec

## Overview

This service provides paid inbound-only mailboxes bound to cryptographic keys.

The stable identity is an `edproof` key. The same key always maps to the same
mailbox. A different key gets a different mailbox. `billing_email` is only a
contact address for invoices or payment links; it is not an identity or
authorization primitive.

The service sells inbound mailbox access only:

- inbound email delivery to the provisioned mailbox address
- read access via IMAP
- optional read access via POP3 in a later phase
- optional read access via HTTP API in a later phase

The service does not sell:

- SMTP submission
- outbound mail sending
- relay access
- sender identity features

## Requirements

### User Stories

- As an agent, I want to present a cryptographic key and get the same mailbox
  every time, so that mailbox identity is stable without creating a user
  account.
- As a billing contact, I want payment links sent to an email address, so that
  a mailbox can be paid without coupling payment to a human account.
- As the system, I want mailbox access to depend on cryptographic proof and
  subscription status, so that only the bound key can retrieve mailbox details.
- As a future integrator, I want mailbox access to be protocol-neutral in the
  core, so that IMAP, POP3, and HTTP API access can share the same policy model.

### Acceptance Criteria

1. WHEN a caller presents a valid `edproof` proof for a previously seen key
   THEN the system SHALL return the same mailbox subscription associated with
   that key.
2. WHEN a caller presents a valid `edproof` proof for a new key THEN the system
   SHALL create a new pending mailbox subscription for that key.
3. WHEN a mailbox subscription is created or renewed THEN the system SHALL send
   the payment link to `billing_email`.
4. WHEN a mailbox subscription is unpaid or expired THEN the system SHALL deny
   mailbox access even if the `edproof` proof is valid.
5. WHEN a caller presents a valid `edproof` proof for the key bound to an
   active mailbox THEN the system SHALL return inbound access details.
6. IF a caller presents a valid `edproof` proof for a different key THEN the
   system SHALL NOT grant access to an existing mailbox bound to another key.
7. WHEN mailbox access is granted THEN the system SHALL return inbound access
   details only and SHALL NOT return SMTP submission details.
8. WHEN future access methods are added THEN the system SHALL apply the same
   key-binding and payment policy across IMAP, POP3, and HTTP API.

## Design

### Product Model

Primary entity: `MailboxSubscription`

- cryptographic mailbox identity is the `edproof` key fingerprint
- commercial contact is `billing_email`
- entitlement is active paid subscription status

Access control model:

- authentication: valid `edproof` proof demonstrating control of key `K`
- authorization: mailbox subscription exists for key `K`
- entitlement: mailbox subscription status is active

### Domain Model

Suggested mailbox subscription fields:

- `id`
- `key_fingerprint` unique
- `key_algorithm`
- `billing_email`
- `mailbox_local_part`
- `mailbox_email`
- `imap_host`
- `imap_port`
- `imap_username`
- `imap_password`
- `status` (`pending_payment`, `active`, `expired`, `revoked`)
- `subscription_expires_at`
- `last_paid_at`
- `stripe_session_id`
- `payment_url`
- `created_at`
- `updated_at`

### HTTP Contract

#### `POST /v1/mailboxes/claim`

Request:

```json
{
  "billing_email": "billing@example.com",
  "edproof": "<proof>"
}
```

Behavior:

- verify proof
- derive stable `key_fingerprint`
- return existing mailbox if key is known
- otherwise create a pending mailbox subscription and payment session

Response:

```json
{
  "mailbox_id": "mbx_123",
  "mailbox_email": "mbx_x@example.com",
  "status": "pending_payment",
  "payment_url": "https://...",
  "subscription_expires_at": null
}
```

#### `POST /v1/mailboxes/renew`

Request:

```json
{
  "billing_email": "billing@example.com",
  "edproof": "<proof>"
}
```

Behavior:

- verify proof
- locate mailbox by `key_fingerprint`
- issue renewal payment session

#### `POST /v1/access/resolve`

Authenticated by `edproof`.

Request:

```json
{
  "protocol": "imap"
}
```

Response:

```json
{
  "mailbox_id": "mbx_123",
  "mailbox_email": "mbx_x@example.com",
  "protocol": "imap",
  "host": "imap.example.com",
  "port": 143,
  "username": "mbx_x",
  "password": "secret"
}
```

`protocol` may later support `pop3`.

### Future HTTP API

Future read-only endpoints should use the same key-bound auth model:

- `GET /v1/messages`
- `GET /v1/messages/{id}`
- `GET /v1/messages/{id}/raw`

These endpoints must remain inbound/read-only and must not expand the product
into outbound sending.

### Architecture

Keep protocol verification in an adapter and business policy in the core.

Suggested new port:

```go
type KeyProofVerifier interface {
    Verify(ctx context.Context, rawProof string) (*VerifiedKey, error)
}

type VerifiedKey struct {
    Fingerprint string
    Algorithm   string
}
```

Suggested service methods:

- `ClaimMailbox(ctx, billingEmail string, key VerifiedKey)`
- `RenewMailbox(ctx, billingEmail string, key VerifiedKey)`
- `ResolveAccess(ctx, key VerifiedKey, protocol string)`
- `ListMessages(ctx, key VerifiedKey, limit int, unreadOnly bool, includeBody bool)`
- `GetMessage(ctx, key VerifiedKey, id string, includeBody bool)`

Suggested adapter:

- `internal/adapters/identity/edproof`

Core service remains protocol-neutral. IMAP, POP3, and HTTP are access adapters
over the same mailbox subscription policy.

### Data Migration

Preferred migration path:

1. add `key_fingerprint` and `billing_email` to `mailboxes`
2. stop relying on `account_id`, `owner_email`, and `access_token` for mailbox access
3. introduce key-based claim and resolve endpoints
4. keep legacy account/token flow temporarily
5. remove account recovery and refresh token flow after migration

### Security Notes

- a valid `edproof` alone grants nothing
- only a valid proof for a key bound to a mailbox grants access
- mailbox access also requires active subscription status
- payment identity is intentionally ignored
- inbound access responses must never include SMTP submission details

## Tasks

- [ ] 1. Introduce key-bound mailbox data model
  - Add `key_fingerprint` and `billing_email` to mailbox persistence.
  - Preserve current mailbox provisioning behavior.
  - _Requirements: 1, 2, 3, 6_

- [ ] 2. Add `edproof` verification port and adapter
  - Create a protocol adapter that verifies proofs and returns `VerifiedKey`.
  - Keep `edproof`-specific types out of the core domain.
  - _Requirements: 1, 2, 5, 6_

- [ ] 3. Implement mailbox claim flow
  - Add `POST /v1/mailboxes/claim`.
  - Reuse existing mailbox for known keys.
  - Create new mailbox subscription for new keys.
  - _Requirements: 1, 2, 3_

- [ ] 4. Implement key-based resolve flow
  - Add `POST /v1/access/resolve`.
  - Verify proof, enforce active subscription, return inbound access details.
  - _Requirements: 4, 5, 7_

- [ ] 5. Keep legacy flow during migration
  - Preserve current account/token endpoints temporarily.
  - Mark them as legacy in docs.
  - _Requirements: 8_

- [ ] 6. Document product boundaries and usage
  - Explain key generation/bootstrap, billing-email role, and inbound-only scope.
  - _Requirements: 3, 7, 8_

- [ ] 7. Design future POP3 and HTTP API access paths
  - Ensure protocol additions reuse the same key-bound authorization model.
  - _Requirements: 8_
