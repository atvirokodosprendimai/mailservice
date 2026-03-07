# AGENTS context

This file gives fast context for coding agents working in this repo.

## Project purpose

Tiny mailbox provisioning API for OpenClaw.

- Mailbox is created for an owner email.
- Owner receives payment link (Stripe or mock).
- After payment, mailbox becomes usable.
- OpenClaw uses API token auth and mailbox access token -> IMAP credentials.

## Architecture

Hexagonal structure:

- Core domain: `internal/domain`
- Core ports: `internal/core/ports`
- Core services: `internal/core/service`
- Adapters:
  - HTTP API: `internal/adapters/httpapi`
  - Repositories (GORM): `internal/adapters/repository`
  - Payment (Stripe/mock): `internal/adapters/payment`
  - Notifier: `internal/adapters/notify`
  - Token generator: `internal/adapters/token`
- Platform:
  - Config: `internal/platform/config`
  - Database + goose migrations: `internal/platform/database`
- Entry point: `cmd/app/main.go`

## Data and persistence

- DB: SQLite without cgo (`github.com/glebarez/sqlite`)
- ORM: GORM
- Migrations: Goose, embedded and executed at startup
- Important tables:
  - `accounts`
  - `mailboxes`
  - `account_recoveries`
  - `refresh_tokens`

## Auth model

Bot/autonomous auth (preferred):

1. `POST /v1/accounts` -> returns `api_token` + `refresh_token` for new account
2. `POST /v1/auth/refresh` -> rotates both tokens

Human fallback only:

- `POST /v1/accounts/recovery/start`
- `POST /v1/accounts/recovery/complete`

Notes:

- Refresh tokens are one-time use and stored hashed.
- Recovery code TTL is 10 minutes.
- Recovery start is rate-limited per account (1 request/minute).

## Concurrency protection

Global non-blocking semaphore middleware exists in HTTP adapter.

- Config: `MAX_CONCURRENT_REQUESTS` from `.env`
- If limit is hit, API returns `503` immediately with:
  - `retry_after_seconds` random in range `3..100`
  - `Retry-After` header

## Main API endpoints

- `POST /v1/accounts`
- `POST /v1/auth/refresh`
- `POST /v1/accounts/recovery/start`
- `POST /v1/accounts/recovery/complete`
- `GET /v1/mailboxes`
- `POST /v1/mailboxes`
- `GET /v1/mailboxes/{id}`
- `POST /v1/imap/resolve`
- `POST /v1/imap/messages` (placeholder)
- `POST /v1/webhooks/stripe`
- `GET /mock/pay/{sessionID}` (dev/mock mode)

## OpenClaw flow

Canonical bot flow is documented in `follow.md`.

## Config

See `.env.example`.

Critical vars:

- `MAX_CONCURRENT_REQUESTS`
- `DATABASE_DSN`
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `PUBLIC_BASE_URL`

## How to run and verify

- Run: `go run ./cmd/app`
- Test: `go test ./...`
- Format: `gofmt -w <files>`

## Agent guardrails

- Keep hexagonal boundaries; do not leak adapter concerns into core domain.
- Prefer adding tests before behavior changes (TDD style).
- Keep API responses stable unless explicitly changing contract.
- Avoid introducing cgo dependencies.
- Do not commit `.env` secrets.
