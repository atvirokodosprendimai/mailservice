# Architecture Overview

## What this system does

`mailservice` provisions paid inbound-only mailboxes.

The product contract is strict:
- same Ed25519 key -> same mailbox
- different key -> different mailbox
- mailbox is usable after confirmed payment
- access is read-only over IMAP (plus HTTP wrappers for reading)
- no SMTP submission or outbound relay

## Architectural style

The codebase follows a hexagonal architecture:

- domain entities in `internal/domain`
- use-case logic in `internal/core/service`
- interfaces in `internal/core/ports`
- infrastructure adapters in `internal/adapters/*`
- app wiring in `cmd/app/main.go`

Core services depend on ports and domain only.
Adapters implement ports and are selected at startup from config.

## Main modules

| Module | Path | Responsibility |
| --- | --- | --- |
| HTTP API adapter | `internal/adapters/httpapi` | Request decoding, auth checks, status mapping, stable JSON API shape. |
| Mailbox service | `internal/core/service/mailbox_service.go` | Claim mailbox, create payment links, activate after payment, resolve access, list/read messages. |
| Account service | `internal/core/service/account_service.go` | Legacy account flow, refresh tokens, recovery flow, account-token auth. |
| Identity adapter | `internal/adapters/identity/edproof` | Verifies Ed25519 public keys, challenge freshness, and signatures. |
| Payment adapters | `internal/adapters/payment` | Polar (preferred), Stripe (legacy fallback), mock provider for local/dev. |
| Repository adapters | `internal/adapters/repository` | GORM persistence for accounts, mailboxes, refresh tokens, recovery, mail runtime provisioning tables. |
| Notifier adapters | `internal/adapters/notify` | Unsend, Resend, SendGrid, Mailgun, log fallback. |
| Mail reader adapter | `internal/adapters/imap` | Reads mailbox messages over IMAP for HTTP endpoints. |
| Platform config/db | `internal/platform/config`, `internal/platform/database` | Runtime config loading, DB initialization, migrations. |

## Runtime composition

`cmd/app/main.go` wires one process with these runtime decisions:

- primary DB mode can be local SQLite or Turso (config-selected)
- local SQLite is always kept for mail runtime tables used by Postfix/Dovecot
- payment provider precedence is Polar -> Stripe -> mock
- notifier can be explicit (`NOTIFIER_PROVIDER`) or fallback cascade
- challenge-response auth is enabled via EdProof authenticator
- global request semaphore is enabled when `MAX_CONCURRENT_REQUESTS > 0`

## Data ownership

- business state (accounts, mailboxes, refresh tokens, recoveries) is persisted through repository ports
- mailbox activation also provisions receive-stack records (`mail_domains`, `mail_users`) via `MailRuntimeProvisioner`
- one-time refresh token semantics are enforced in service + repository behavior

## Key security boundaries

- challenge-response verification gates key-bound claim and key-bound access resolve
- webhook endpoints require signature verification (Stripe/Polar)
- request bodies are size-limited and unknown JSON fields are rejected
- account and admin APIs use separate token paths (`X-API-Token`/Bearer vs admin bearer key)

## Compatibility boundary

Current migration state keeps both flows active:

- preferred flow: `POST /v1/mailboxes/claim` -> payment -> `POST /v1/access/resolve`
- legacy flow: account + token + mailbox endpoints

When changing architecture, keep legacy endpoints stable unless migration policy explicitly changes.
