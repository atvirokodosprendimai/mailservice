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

- Second systemd service (`mailservice-api-smoke`) on port 8081
- Separate SQLite DB (`/var/lib/mailservice/data/mailservice-smoke.db`)
- Separate env file (`/var/lib/secrets/mailservice-smoke.env`)
- Shared Postfix/Dovecot/IMAP stack (same mail domain, smoke mailboxes are regular mailboxes)
- Polar sandbox config (`sandbox-api.polar.sh`, sandbox product, sandbox webhook secret)
- Same binary artifact — only config differs

## Phases

### Phase 1 - Polar sandbox product setup - status: open

1. [ ] Create sandbox product on Polar matching production config
   - use Polar sandbox API with `POLAR_SANDBOX_TOKEN`
   - product name: "Mailbox (smoke test)"
   - price: 1 EUR/month (matches production)
   - capture the sandbox product ID
2. [ ] Create sandbox webhook endpoint on Polar
   - point to `https://truevipaccess.com/smoke/v1/webhooks/polar` (or port-based routing)
   - capture the webhook secret
3. [ ] Set GitHub secrets/variables for sandbox config
   - `POLAR_SANDBOX_PRODUCT_ID` (variable)
   - `POLAR_SANDBOX_WEBHOOK_SECRET` (secret)
   - `POLAR_SANDBOX_SERVER_URL` = `https://sandbox-api.polar.sh` (variable)

### Phase 2 - NixOS smoke test instance - status: open

1. [ ] Add `mailservice-api-smoke` systemd service to NixOS module
   - same binary (`cfg.package`), different env file and DB path
   - listen on `127.0.0.1:8081`
   - `DATABASE_DSN` = `/var/lib/mailservice/data/mailservice-smoke.db`
   - `EnvironmentFile` = `/var/lib/secrets/mailservice-smoke.env`
   - separate tmpfiles rules for smoke DB directory
2. [ ] Add nginx route for smoke instance
   - `/smoke/` prefix proxied to `127.0.0.1:8081` (strip prefix)
   - or subdomain — decide based on webhook URL constraints
3. [ ] Update deploy workflow to write `mailservice-smoke.env`
   - uses `POLAR_SANDBOX_TOKEN`, `POLAR_SANDBOX_PRODUCT_ID`, etc.
   - same `MAIL_DOMAIN`, `IMAP_HOST`, `IMAP_PORT` as production
   - `PUBLIC_BASE_URL` set to the smoke instance URL

### Phase 3 - Smoke test script with auto-payment - status: open

1. [ ] Update `ops/smoke-test-periodic.sh` to support auto-payment mode
   - new flag: `--polar-token` / `SMOKE_POLAR_TOKEN` env var
   - new flag: `--polar-api` / `SMOKE_POLAR_API` env var (default: `https://sandbox-api.polar.sh`)
   - when set: after claim, GET checkout by ID from Polar API → get `client_secret`
   - PATCH checkout with customer email + confirm via `POST /v1/checkouts/client/{client_secret}/confirm`
   - if confirm needs Stripe details, fall back to updating checkout + triggering success URL
   - poll resolve until active (short timeout, e.g. 30s)
2. [ ] Test auto-payment flow against sandbox instance
   - claim → auto-pay → webhook → activate → resolve → IMAP → messages
3. [ ] Update GitHub Actions workflow
   - point at smoke instance URL
   - pass `POLAR_SANDBOX_TOKEN` for auto-payment
   - remove key caching (no longer needed — fresh key each run)

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
- Smoke test mailboxes use separate DB, don't pollute production data

## Adjustments

## Progress Log
