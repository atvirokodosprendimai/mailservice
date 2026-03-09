# Mail Service API

Tiny hexagonal Go API for paid inbound mailbox provisioning.

Current preferred flow:
1. Agent presents `billing_email` plus key proof to `POST /v1/mailboxes/claim`.
2. Service reuses the same mailbox for the same key, or creates a new pending mailbox for a new key.
3. Service sends payment link to `billing_email`.
4. After payment, mailbox becomes active for one month.
5. Agent presents the same key proof to `POST /v1/access/resolve` to obtain IMAP access details.

Legacy flow remains available during migration:
- account creation via `POST /v1/accounts`
- account token refresh via `POST /v1/auth/refresh`
- mailbox creation via `POST /v1/mailboxes`
- IMAP resolve via `POST /v1/imap/resolve`

Product scope:
- inbound mailbox access only
- IMAP today
- POP3 / HTTP read API later
- no SMTP submission or outbound sending

Further reading:
- [Architecture docs](docs/architecture/README.md)
- [Key-bound mailbox spec](docs/key-bound-mailbox-spec.md)
- [Use cases](docs/use-cases.md)
- [Website copy](docs/website-copy.md)
- [Migration plan](docs/migration-plan.md)
- [Future access design](docs/future-access.md)
- [Hetzner CI/CD](docs/hetzner-cicd.md)
- [Hetzner NixOS snapshot builder](docs/hetzner-nixos-snapshot.md)
- [NixOS GitOps on Hetzner](docs/nixos-gitops.md)
- [NixOps migration spec](docs/nixops-migration-spec.md)
- [NixOps migration plan](docs/nixops-migration-plan.md)
- [Local workflow validation](docs/local-workflow-validation.md)
- [truevipaccess.com deployment](docs/truevipaccess-deploy.md)
- [Cloudflare Tunnel deployment](docs/cloudflare-tunnel-deploy.md)

Deployment runtime template:
- [production.env.example](deploy/production.env.example)

## Stack

- Go
- Hexagonal architecture (`internal/core` ports/services + adapter packages)
- SQLite (pure Go, no CGO) via `github.com/glebarez/sqlite`
- GORM ORM
- Goose SQL migrations
- Polar checkout for the preferred key-bound flow
- Stripe Checkout + webhooks kept as legacy fallback
- mock payment links for local development

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

One-command local stack from GHCR images:

```bash
cp compose.yml.example compose.yml
docker compose pull
docker compose up -d
docker compose logs -f mailreceive
```

Tunnel-based production compose baseline:

```bash
cp compose.tunnel.yml.example compose.tunnel.yml
docker compose -f compose.tunnel.yml pull
docker compose -f compose.tunnel.yml up -d
```

The service auto-loads `.env` from the project root (via `godotenv`).

Production delivery:
- production runs on a NixOS host with native API, Postfix, Dovecot, and cloudflared services
- merges to `main` trigger `Deploy Production App`
- CI builds the NixOS system closure first and can push it to Hetzner S3 binary cache when configured
- the deploy workflow syncs the repo to the host and runs `nixos-rebuild switch --flake .#truevipaccess`
- `Hetzner OpenTofu` remains the manual workflow for infrastructure changes

Live smoke test helper:

```bash
./ops/smoke-test-mailbox.sh --billing-email you@example.com
```

The script:
- checks `/healthz`
- generates an Ed25519 key pair if needed
- claims a mailbox
- prints the payment URL
- polls `/v1/access/resolve` until payment activates the mailbox

By default it sends the contents of `<work-dir>/identity.pub` as the `edproof` payload,
or `<key-path>.pub` when a custom key path is used.
If your verifier expects a different proof blob, pass `--edproof` or `--edproof-file`.

On the NixOS production host:
- the API runs as a native systemd service built by Nix
- Postfix and Dovecot handle inbound mail and IMAP as native NixOS services

## Environment variables

- `HTTP_ADDR` (default `:8080`)
- `DATABASE_DSN` (default `mailservice.db`)
- `MAX_CONCURRENT_REQUESTS` (default `100`, set `0` to disable semaphore)
- `PUBLIC_BASE_URL` (default `http://localhost:8080`)
- `MAIL_DOMAIN` (default `mail.local`)
- `IMAP_HOST` (default `MAIL_DOMAIN`)
- `IMAP_PORT` (default `143`)
- `MAILBOX_PRICE_CENTS` (default `299`)
- `POLAR_TOKEN` (optional; enable Polar for the preferred key-bound flow)
- `POLAR_PRICE_ID` (required when Polar is enabled)
- `POLAR_SERVER_URL` (default `https://api.polar.sh`)
- `POLAR_SUCCESS_URL` (default `PUBLIC_BASE_URL/v1/payments/polar/success?checkout_id={CHECKOUT_ID}`)
- `POLAR_RETURN_URL` (default `PUBLIC_BASE_URL`)
- `POLAR_WEBHOOK_SECRET` (recommended for production; enables signed `POST /v1/webhooks/polar`)
- `STRIPE_CURRENCY` (default `usd`)
- `STRIPE_SUCCESS_URL` (default `http://localhost:8080/payment/success`)
- `STRIPE_CANCEL_URL` (default `http://localhost:8080/payment/cancel`)
- `STRIPE_SECRET_KEY` (optional legacy fallback; if no real provider is configured, mock payment links are used)
- `STRIPE_WEBHOOK_SECRET` (required only for real Stripe webhook verification)
- `SENDGRID_API_KEY` (optional; enable SendGrid notifier)
- `SENDGRID_FROM_EMAIL` (required when SendGrid is enabled)
- `SENDGRID_FROM_NAME` (optional, default `MailService`)
- `RESEND_API_KEY` (optional; enable Resend notifier)
- `RESEND_FROM_EMAIL` (required when Resend is enabled)
- `RESEND_FROM_NAME` (optional, default `MailService`)

When both providers are configured, Resend takes precedence.

## API examples

Preferred key-bound claim flow:

```bash
curl -X POST http://localhost:8080/v1/mailboxes/claim \
  -H 'Content-Type: application/json' \
  -d '{"billing_email":"billing@example.com","edproof":"<proof>"}'
```

Confirm Polar payment after redirect fallback:

```bash
curl "http://localhost:8080/v1/payments/polar/success?checkout_id=<polar-checkout-id>"
```

Preferred production payment completion path:

```bash
curl -X POST http://localhost:8080/v1/webhooks/polar \
  -H 'webhook-id: <message-id>' \
  -H 'webhook-timestamp: <unix-seconds>' \
  -H 'webhook-signature: v1,<signature>' \
  -d '<signed-payload-from-polar>'
```

Resolve IMAP credentials by key proof:

```bash
curl -X POST http://localhost:8080/v1/access/resolve \
  -H 'Content-Type: application/json' \
  -d '{"protocol":"imap","edproof":"<proof>"}'
```

If global concurrency limit is reached, API returns `503` with `retry_after_seconds` random value in range `3..100`.

Legacy account flow:

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

Fetch unread messages by access token:

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<access-token>","unread_only":true,"limit":20,"include_body":false}'
```

`unread_only` defaults to `true`; `include_body` defaults to `false`.

Fetch a single message by UID:

```bash
curl -X POST http://localhost:8080/v1/imap/messages/get \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<access-token>","uid":1,"include_body":true}'
```

For `messages/get`, `include_body` defaults to `true`.

Mock payment (only when Stripe key is not configured):

```bash
curl http://localhost:8080/mock/pay/<session-id>
```

## Notes

- The same key always maps to the same mailbox.
- A different key gets a different mailbox.
- `billing_email` is only the address used for invoice/payment delivery.
- Who actually pays is outside the service model.
