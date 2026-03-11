---
tldr: Full architectural snapshot of mailservice — hexagonal Go service for paid encrypted mailboxes with Ed25519 identity, NixOS GitOps deployment
category: observation
---

# Architecture: Full Codebase Snapshot

## Git State

- **Branch:** `main`
- **Commit:** `612af45` — Merge branch 'task/compound-p2-security-doc'
- **Active branches:** ~90 branches (mix of `task/`, `feat/`, `fix/`, `infra/`, `nix/`, `ci/`, `docs/`)

## Directory Structure

```
mailservice/
  cmd/app/                  # Application entrypoint (main.go)
  internal/
    adapters/
      httpapi/              # HTTP handler, routes, middleware (1305 lines)
      identity/edproof/     # Ed25519 challenge-response auth (challenge.go, verifier.go)
      imap/                 # IMAP mail reader
      notify/               # Email notifiers (mailgun, resend, sendgrid, unsend, log)
      payment/              # Payment gateways (polar, stripe, mock)
      repository/           # GORM-backed repositories (mailbox, account, mail_runtime)
      token/                # Secure token generator
    core/
      ports/                # Port interfaces (ports.go — all contracts)
      service/              # Business logic (mailbox_service.go, account_service.go)
    domain/                 # Domain models (mailbox, account, account_recovery, refresh_token)
    platform/
      config/               # Environment config loader
      database/             # SQLite + Turso database init and migrations
  deploy/                   # production.env.example
  docker/                   # Dockerfile support
  docs/                     # Specs, plans, solutions, architecture docs
  eidos/                    # Spec-driven development files
  infra/opentofu/           # Infrastructure as code (Hetzner)
  memory/                   # Plans, decisions, architecture snapshots
  nix/
    hosts/                  # NixOS host configs (truevipaccess, smoke)
    modules/                # mailservice-gitops.nix
  tests/                    # Integration/smoke tests
  todos/                    # Tracked review findings
  .github/workflows/        # CI/CD (9 workflows)
  flake.nix                 # Nix flake (build, dev shell)
```

## Tech Stack

- **Language:** Go 1.25
- **Database:** SQLite (local, for Postfix/Dovecot) + optional Turso (remote, for app data)
- **ORM:** GORM
- **Migrations:** Goose v3
- **IMAP:** go-imap v1
- **Payments:** Polar (primary), Stripe (legacy), Mock
- **Email:** Mailgun (primary), Resend, SendGrid, Unsend, Log
- **Identity:** Ed25519 SSHSIG challenge-response (stateless HMAC-authenticated)
- **Infrastructure:** NixOS on Hetzner, Cloudflare tunnel, OpenTofu
- **CI/CD:** GitHub Actions → NixOS GitOps deploy
- **Build:** Nix flake (`flake.nix`)

## Architecture Pattern

**Hexagonal (ports & adapters).**

```
                    ┌─────────────────────┐
  HTTP requests ──▶ │    httpapi/handler   │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │   core/service       │
                    │  (mailbox, account)  │
                    └──────────┬──────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
  ┌───────▼───────┐  ┌────────▼────────┐  ┌───────▼───────┐
  │  repository   │  │    payment      │  │    notify     │
  │  (GORM/SQLite │  │ (Polar/Stripe)  │  │ (Mailgun/…)   │
  │   /Turso)     │  │                 │  │               │
  └───────────────┘  └─────────────────┘  └───────────────┘
```

All adapter dependencies are injected via port interfaces defined in `core/ports/ports.go`.

## Domain Models

### Mailbox
States: `pending_payment` → `active` → `expired`.
Fields: ID, AccountID, OwnerEmail, KeyFingerprint, IMAP credentials, AccessToken, PaymentSessionID, PaidAt, ExpiresAt.
Method: `Usable()` — active + paid + not expired.

### Account
Fields: ID, OwnerEmail, APIToken, SubscriptionExpiresAt.
Method: `SubscriptionActive(now)`.

### AccountRecovery
One-time recovery codes with expiry and used-at tracking.

### RefreshToken
Token rotation with hash-based lookup and used-at tracking.

## Port Interfaces

All in `internal/core/ports/ports.go`:

| Port | Implementations |
|------|----------------|
| `MailboxRepository` | `repository.NewMailboxRepository` (GORM) |
| `AccountRepository` | `repository.NewAccountRepository` (GORM) |
| `AccountRecoveryRepository` | `repository.NewAccountRecoveryRepository` (GORM) |
| `RefreshTokenRepository` | `repository.NewRefreshTokenRepository` (GORM) |
| `PaymentGateway` | `payment.PolarGateway`, `payment.StripeGateway`, `payment.MockGateway` |
| `Notifier` | `notify.MailgunNotifier`, `notify.ResendNotifier`, `notify.SendGridNotifier`, `notify.UnsendNotifier`, `notify.LogNotifier` |
| `TokenGenerator` | `token.SecureGenerator` |
| `KeyProofVerifier` | `edproof.Verifier` |
| `MailRuntimeProvisioner` | `repository.MailRuntimeProvisioner` (writes Postfix/Dovecot SQLite tables) |
| `MailReader` | `imap.Reader` |

## Entry Points

- **`cmd/app/main.go`** — Bootstrap: load config → open databases → create adapters → inject into services → inject into handler → start HTTP server with graceful shutdown.
- **`flake.nix`** — Nix build definition, dev shell.
- **`nix/modules/mailservice-gitops.nix`** — NixOS service module.
- **`.github/workflows/deploy-production.yml`** — CI/CD: build via Nix, deploy to Hetzner, health check.

## HTTP API (handler.go, 1305 lines)

Key routes (from handler patterns):
- `GET /` — Homepage with agent instructions, ETag caching
- `POST /v1/mailboxes` — Claim a mailbox (key-bound or token-bound)
- `GET /v1/mailboxes/{id}` — Get mailbox status
- `POST /v1/mailboxes/{id}/resolve` — Resolve mailbox access (Ed25519 challenge-response)
- `GET /v1/mailboxes/{id}/messages` — List messages via IMAP
- `GET /v1/mailboxes/{id}/messages/{uid}` — Read single message
- `POST /v1/challenges` — Generate Ed25519 challenge
- `POST /v1/accounts` — Create account
- `POST /v1/accounts/recover` — Initiate recovery
- `POST /v1/webhooks/stripe` — Stripe payment webhook
- `POST /v1/webhooks/polar` — Polar payment webhook
- Admin endpoints (admin API key auth)

Security features:
- Constant-time admin key comparison (`subtle.ConstantTimeCompare`)
- Request body size limits (1 MB via `io.LimitReader`)
- HTML escaping in email templates
- HMAC-authenticated stateless challenges (30s TTL)
- Concurrency limiter middleware

## Authentication Flows

### Ed25519 Challenge-Response (edproof)
1. Client requests challenge: `POST /v1/challenges` with public key
2. Server returns HMAC-signed challenge (nonce + fingerprint + expiry)
3. Client signs challenge with private key using `ssh-keygen -Y sign`
4. Client submits signature: `POST /v1/mailboxes/{id}/resolve` with challenge + signature
5. Server verifies HMAC, checks expiry, verifies SSHSIG signature
6. Returns IMAP credentials on success

### Account Recovery
Owner email → recovery link → one-time code → new API token.

## Database Strategy

- **Local SQLite** (`/data/mailservice.db`): Always active. Postfix/Dovecot mail_users and mail_domains tables are written here by `MailRuntimeProvisioner`.
- **Turso** (optional): When `DATABASE_MODE=turso`, app data (mailboxes, accounts) goes to Turso. Mail runtime tables stay in local SQLite.
- **Migrations:** Goose v3, embedded in the binary.

## Deployment

- **Target:** Single Hetzner VPS running NixOS
- **Method:** GitHub Actions → Nix build → `nixos-rebuild switch` via SSH
- **Networking:** Cloudflare tunnel (no direct port exposure)
- **TLS:** ACME certificates for mail services (Postfix, Dovecot)
- **Health check:** `GET /healthz` verified post-deploy

## CI/CD Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `deploy-production.yml` | push to main | Build + deploy to Hetzner |
| `deploy-smoke.yml` | manual | Deploy smoke test instance |
| `smoke-test-periodic.yml` | schedule | Periodic smoke tests |
| `db-check.yml` | manual | Database health checks |
| `turso-seed.yml` | manual | Seed Turso database |
| `polar-setup-webhook.yml` | manual | Configure Polar webhook |
| `hetzner-nixos-snapshot.yml` | manual | Create server snapshot |
| `hetzner-opentofu.yml` | manual | Infrastructure provisioning |
| `hetzner-server-reboot.yml` | manual | Server reboot |

## Patterns

- **Hexagonal architecture** with strict port/adapter separation
- **Constructor injection** — all dependencies wired in `main.go`
- **Explicit provider selection** — notifier and payment gateway chosen by config, not implicit cascade (cascade is deprecated)
- **Stateless authentication** — HMAC-signed challenges avoid server-side session storage
- **Immutable domain models** — `Usable()` and `SubscriptionActive()` are pure functions on value types
- **Table-driven tests** — consistent across all test files

## Notes

- `handler.go` at 1305 lines is the largest file — contains all HTTP routes and middleware in one file
- Legacy Stripe gateway exists alongside Polar (primary payment provider)
- Notifier cascade (`selectNotifierCascade`) is deprecated but still present as fallback
- ~90 git branches accumulated; many are completed task branches that were never deleted (per convention: "never delete branches after merging")
