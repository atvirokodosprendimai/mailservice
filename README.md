# Mail Service API

Tiny hexagonal Go API for paid mailbox provisioning.

Flow:
1. OpenClaw creates account (`POST /v1/accounts`) once and gets `api_token` + `refresh_token`.
2. OpenClaw refreshes credentials autonomously via `POST /v1/auth/refresh`.
3. Owner email recovery remains human-only fallback when all bot credentials are lost.
4. OpenClaw lists mailboxes (`GET /v1/mailboxes`).
5. OpenClaw creates mailbox (`POST /v1/mailboxes`).
6. Service creates Stripe Checkout link and sends it to owner email (Resend or SendGrid when configured, log fallback otherwise).
7. Owner pays.
8. Stripe webhook extends account subscription by 1 month; all account mailboxes inherit entitlement.
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

## Docker

Build API service image:

```bash
docker build -t mailservice-api:latest -f Dockerfile .
```

Build receive-only mail service image (Postfix + Dovecot + SQLite):

```bash
docker build -t mailservice-receive:latest -f docker/mailreceive/Dockerfile .
```

Run receive-only mail service:

```bash
docker run --rm -p 25:25 -p 143:143 \
  -v "$(pwd)/mailservice.db:/data/mailservice.db" \
  -e MAIL_DOMAIN=mail.local \
  -e MAILBOX_USER=test \
  -e MAILBOX_PASSWORD=secret \
  mailservice-receive:latest
```

The receive container can share the same SQLite DB used by the API (`/data/mailservice.db`).
API writes mailbox provisioning records into `mail_domains` and `mail_users` on payment activation.

One-command local stack:

```bash
docker compose up --build
```

The service auto-loads `.env` from the project root (via `godotenv`).

## Environment variables

- `HTTP_ADDR` (default `:8080`)
- `DATABASE_DSN` (default `mailservice.db`)
- `MAX_CONCURRENT_REQUESTS` (default `100`, set `0` to disable semaphore)
- `PUBLIC_BASE_URL` (default `http://localhost:8080`)
- `MAILBOX_PRICE_CENTS` (default `299`)
- `STRIPE_CURRENCY` (default `usd`)
- `STRIPE_SUCCESS_URL` (default `http://localhost:8080/payment/success`)
- `STRIPE_CANCEL_URL` (default `http://localhost:8080/payment/cancel`)
- `STRIPE_SECRET_KEY` (optional; if empty, mock payment links are used)
- `STRIPE_WEBHOOK_SECRET` (required only for real Stripe webhook verification)
- `SENDGRID_API_KEY` (optional; enable SendGrid notifier)
- `SENDGRID_FROM_EMAIL` (required when SendGrid is enabled)
- `SENDGRID_FROM_NAME` (optional, default `MailService`)
- `RESEND_API_KEY` (optional; enable Resend notifier)
- `RESEND_FROM_EMAIL` (required when Resend is enabled)
- `RESEND_FROM_NAME` (optional, default `MailService`)

When both providers are configured, Resend takes precedence.

## API examples

Create account:

```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Refresh machine credentials:

```bash
curl -X POST http://localhost:8080/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh-token>"}'
```

If global concurrency limit is reached, API returns `503` with `retry_after_seconds` random value in range `3..100`.

Human-only recovery start endpoint:

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/start \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Human recovery complete by URL token (browser friendly):

```bash
open "http://localhost:8080/v1/accounts/recovery/complete?token=<one-time-token>"
```

Complete token recovery by POST token:

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/complete \
  -H 'Content-Type: application/json' \
  -d '{"token":"<one-time-token>"}'
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
