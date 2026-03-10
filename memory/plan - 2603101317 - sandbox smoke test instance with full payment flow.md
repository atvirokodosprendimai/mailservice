---
tldr: Deploy second mailservice instance with Polar sandbox and separate DB for automated E2E smoke testing every 5 minutes
status: active
---

# Plan: Sandbox smoke test instance with full payment flow

## Context

- Todo: [[todo - 2603101248 - periodic smoke test using Polar sandbox for full claim to read cycle]]
- Existing smoke test: `ops/smoke-test-periodic.sh` (post-payment only, reuses paid key)
- NixOS module: `nix/modules/mailservice-gitops.nix`
- Polar webhook handler: `internal/adapters/httpapi/handler.go:954`
- `POLAR_SANDBOX_TOKEN` GitHub secret already set

## Architecture

- **Separate server** (second Hetzner instance) — fully isolated from production
- Separate domain (e.g. `smoke.truevipaccess.com` or `test-mail.truevipaccess.com`)
- Own Postfix/Dovecot/IMAP stack, own SQLite DB, own ACME certs
- Own env file with Polar sandbox config (`sandbox-api.polar.sh`)
- **Same NixOS flake and binary artifacts** — only config differs
- Deployed from the same CI/CD pipeline (same GitHub Actions, different host target)

## Phases

### Phase 1 - Polar sandbox product setup - status: open

1. [x] Create sandbox product on Polar matching production config
   - use Polar sandbox API with `POLAR_SANDBOX_TOKEN`
   - product name: "Mailbox (smoke test)"
   - price: 1 EUR/month (matches production)
   - capture the sandbox product ID
   - => already existed: "TrueVip Mailbox" `bc05e94d-22f7-43b4-8a90-75d1062a6923` — 1 EUR/month recurring, EUR, not archived
2. [ ] Create sandbox webhook endpoint on Polar
   - point to `https://truevipaccess.com/smoke/v1/webhooks/polar` (or port-based routing)
   - capture the webhook secret
3. [ ] Set GitHub secrets/variables for sandbox config
   - `POLAR_SANDBOX_PRODUCT_ID` (variable)
   - `POLAR_SANDBOX_WEBHOOK_SECRET` (secret)
   - `POLAR_SANDBOX_SERVER_URL` = `https://sandbox-api.polar.sh` (variable)

### Phase 2 - Smoke test server infrastructure - status: open

1. [x] Provision a second Hetzner server for smoke tests
   - small instance (CX22 or similar — minimal load)
   - add to OpenTofu config alongside production server
   - => OpenTofu updated with `cloudflare_zone_name` var (for subdomain zone lookup) and `create_domain_a_record` (for direct nginx access). Same TF, different state via `smoke` GitHub environment. Hetzner OpenTofu workflow updated with new inputs.
   - => **server not yet created** — need to create `smoke` GitHub environment, set secrets/vars, then run OpenTofu workflow with `environment=smoke`, `name=mailservice-smoke`, `cloudflare_zone_name=truevipaccess.com`, `create_domain_a_record=true`
2. [x] Create DNS records for smoke domain
   - e.g. `smoke.truevipaccess.com` A record → smoke server IP
   - MX record for the smoke domain → `mail.smoke.truevipaccess.com`
   - `mail.smoke.truevipaccess.com` A record → smoke server IP
   - => handled by OpenTofu: `domain_a` (conditional), `mail_a`, `mx_primary` resources. Will be created when OpenTofu runs.
3. [x] Add NixOS host config for smoke server
   - `nix/hosts/smoke/configuration.nix` — same flake, different host
   - reuses `mailservice-gitops` module with `mailDomain = "smoke.truevipaccess.com"`
   - same binary artifact from flake
   - => created `nix/hosts/smoke/configuration.nix` and `hardware-configuration.nix`
   - => disables cloudflared, adds nginx reverse proxy for `smoke.truevipaccess.com` with ACME + forceSSL
   - => added `nixosConfigurations.smoke` to `flake.nix`
4. [x] Add deploy workflow for smoke server
   - separate workflow or job in existing workflow
   - writes `mailservice.env` with sandbox Polar config
   - `POLAR_TOKEN` = `POLAR_SANDBOX_TOKEN`
   - `POLAR_PRODUCT_ID` = `POLAR_SANDBOX_PRODUCT_ID`
   - `POLAR_WEBHOOK_SECRET` = `POLAR_SANDBOX_WEBHOOK_SECRET`
   - `POLAR_SERVER_URL` = `https://sandbox-api.polar.sh`
   - `PUBLIC_BASE_URL` = `https://smoke.truevipaccess.com`
   - `MAIL_DOMAIN` = `smoke.truevipaccess.com`
   - => created `.github/workflows/deploy-smoke.yml` — separate workflow, `smoke` GitHub environment, deploys on push to main + workflow_dispatch

### Phase 3 - Smoke test script with auto-payment - status: open

1. [x] Update `ops/smoke-test-periodic.sh` to support auto-payment mode
   - new flag: `--polar-token` / `SMOKE_POLAR_TOKEN` env var
   - new flag: `--polar-api` / `SMOKE_POLAR_API` env var (default: `https://sandbox-api.polar.sh`)
   - new flag: `--auto-pay` / `SMOKE_AUTO_PAY` env var
   - => Stripe blocks API tokenization — pivoted to free sandbox product. Checkout confirms without Stripe, webhook still fires.
   - => free sandbox product created: `ce03e78f-930b-4693-93a0-6b0ff67aff7c` ("Mailbox Smoke Test (Free)")
   - => extracts `client_secret` from `payment_url` (last path segment), confirms via POST to Polar client API
   - => polls claim endpoint until mailbox activates (30s timeout, 2s interval)
   - => fresh Ed25519 key generated each run in auto-pay mode
2. [ ] Test auto-payment flow against sandbox instance
   - claim → auto-pay → webhook → activate → resolve → IMAP → messages
   - => blocked on smoke server being provisioned
3. [x] Update GitHub Actions workflow
   - point at smoke instance URL
   - pass `POLAR_SANDBOX_TOKEN` for auto-payment
   - remove key caching (no longer needed — fresh key each run)
   - => workflow runs both `smoke-production` (persistent key, production env) and `smoke-sandbox` (auto-pay, smoke env) jobs

### Phase 4 - Cleanup and verification - status: open

1. [ ] Verify the full cycle runs green in CI
   - trigger workflow manually, confirm 5/5 checks pass
2. [ ] Let the cron run for 30 minutes, check for flakiness
3. [ ] Update the todo to solved
4. [ ] Remove or archive the old key-caching smoke test approach if superseded

## Verification

- GitHub Actions `Periodic Smoke Test` workflow runs green every 5 minutes
- Each run exercises: healthz → claim → pay (sandbox) → activate → resolve → IMAP login → HTTP API
- No manual payment needed — fully automated
- Smoke test runs on a completely separate server with its own domain — zero production impact

## Adjustments

- **2603101317** — initial plan had shared server with separate DB
- **2603101330** — revised: fully separate server with own domain, own mail stack, zero production coupling

## Progress Log

- **2603101445** — Phase 1.1: sandbox product already exists (`bc05e94d-22f7-43b4-8a90-75d1062a6923`), marked complete
- **2603101500** — Phase 2: OpenTofu, NixOS host, flake, and deploy workflow created for smoke server. Infrastructure code complete — actual provisioning requires creating `smoke` GitHub environment with secrets/vars
- **2603101730** — Phase 3: smoke test script updated with auto-pay mode. Free sandbox product (`ce03e78f`). Workflow updated with dual jobs. Testing blocked on server provisioning.
