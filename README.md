# Mail Service API

Tiny hexagonal Go API for paid mailbox provisioning.

Flow:
1. OpenClaw creates account token (`POST /v1/accounts`) for new owner email.
2. If account exists and token is lost, OpenClaw starts email-based recovery (`POST /v1/accounts/recovery/start`).
3. Owner mailbox receives one-time recovery code; OpenClaw reads it and completes recovery (`POST /v1/accounts/recovery/complete`).
4. OpenClaw lists mailboxes (`GET /v1/mailboxes`).
5. OpenClaw creates mailbox (`POST /v1/mailboxes`).
6. Service creates Stripe Checkout link and sends it to owner email (currently logged by notifier adapter).
7. Owner pays.
8. Stripe webhook marks mailbox active.
9. OpenClaw polls mailbox status (`GET /v1/mailboxes/{id}`) and receives `access_token` once usable.
10. OpenClaw resolves token to IMAP login (`POST /v1/imap/resolve`) or fetches messages (`POST /v1/imap/messages`).

## Stack

- Go
- Hexagonal architecture (`internal/core` ports/services + adapter packages)
- SQLite (pure Go, no CGO) via `github.com/glebarez/sqlite`
- GORM ORM
- Goose SQL migrations
- Stripe Checkout + webhooks

## Run

```bash
go run ./cmd/app
```

The service auto-loads `.env` from the project root (via `godotenv`).

## Environment variables

- `HTTP_ADDR` (default `:8080`)
- `DATABASE_DSN` (default `mailservice.db`)
- `PUBLIC_BASE_URL` (default `http://localhost:8080`)
- `MAILBOX_PRICE_CENTS` (default `299`)
- `STRIPE_CURRENCY` (default `usd`)
- `STRIPE_SUCCESS_URL` (default `http://localhost:8080/payment/success`)
- `STRIPE_CANCEL_URL` (default `http://localhost:8080/payment/cancel`)
- `STRIPE_SECRET_KEY` (optional; if empty, mock payment links are used)
- `STRIPE_WEBHOOK_SECRET` (required only for real Stripe webhook verification)

## API examples

Create account (new owner email):

```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Start token recovery (always returns accepted):

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/start \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Complete token recovery with one-time code from owner inbox:

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/complete \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com","code":"<recovery-code>"}'
```

List mailboxes:

```bash
curl http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

Create mailbox:

```bash
curl -X POST http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

Check mailbox status:

```bash
curl http://localhost:8080/v1/mailboxes/<mailbox-id> \
  -H 'X-API-Token: <api-token>'
```

Resolve IMAP credentials by access token:

```bash
curl -X POST http://localhost:8080/v1/imap/resolve \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<access-token>"}'
```

Fetch messages by access token (placeholder endpoint):

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<access-token>"}'
```

Mock payment (only when Stripe key is not configured):

```bash
curl http://localhost:8080/mock/pay/<session-id>
```
