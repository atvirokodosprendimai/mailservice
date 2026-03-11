# Runtime Sequences

This page captures request-level behavior that complements the C4 views.

## Preferred claim and activation flow

1. Agent requests challenge: `POST /v1/auth/challenge` with `public_key`.
2. API creates a short-lived signed challenge.
3. Agent signs challenge with Ed25519 private key.
4. Agent claims mailbox: `POST /v1/mailboxes/claim` with `billing_email`, `edproof`, `challenge`, `signature`.
5. API verifies challenge freshness and signature, then verifies key proof and derives key fingerprint.
6. Mailbox service reuses existing mailbox by key fingerprint or creates a pending mailbox.
7. Payment adapter creates checkout session and notifier delivers payment link to `billing_email`.
8. Payment success is confirmed by webhook (`POST /v1/webhooks/polar` or `POST /v1/webhooks/stripe`) or Polar success callback (`GET /v1/payments/polar/success`).
9. Mailbox service marks mailbox active and provisions runtime mailbox records.
10. Agent requests new challenge, signs it, and calls `POST /v1/access/resolve` to receive IMAP access details and `access_token`.

## Key checks in the flow

- challenge TTL is 30 seconds
- challenge and signature are required for key-bound claim/resolve
- payment must be confirmed/succeeded before mailbox is usable
- unconfirmed mailbox access returns conflict status (`waiting_payment`)

## Legacy account flow (kept during migration)

1. Create account: `POST /v1/accounts`.
2. Refresh with one-time refresh token: `POST /v1/auth/refresh`.
3. Create/list/get mailbox with account token endpoints.
4. Resolve IMAP by mailbox `access_token` using `POST /v1/imap/resolve`.

## IMAP read API flow

1. Client calls `POST /v1/imap/messages` or `POST /v1/imap/messages/get` with `access_token`.
2. Mailbox service validates mailbox usability and resolves host/user/password.
3. IMAP adapter executes mailbox read.
4. API returns normalized JSON response (`provider: imap`).

## Failure mapping conventions

- invalid challenge/signature/key proof -> `401`
- unknown mailbox/message -> `404`
- unpaid mailbox -> `409` with `{"status":"waiting_payment"}`
- semaphore saturation -> `503` with retry headers/body
- malformed request -> `400`
