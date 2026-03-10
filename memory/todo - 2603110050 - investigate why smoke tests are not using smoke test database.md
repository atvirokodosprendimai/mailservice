---
tldr: Smoke test instances may not be using an isolated database — investigate data flow
category: ops
---

# Todo: Investigate why smoke tests are not using smoke test database

## Context

- Smoke server (`smoke.truevipaccess.com`) deploys without `TURSO_DATABASE_URL`/`TURSO_AUTH_TOKEN` — uses local SQLite at default `mailservice.db`
- Production server uses Turso cloud database
- Smoke test workflow (`smoke-test-periodic.yml`) hits `smoke` environment's `PUBLIC_BASE_URL`
- Each smoke run claims a new mailbox with a fresh Ed25519 key (auto-pay mode)

## Questions to investigate

1. Is the smoke server correctly using its own local SQLite (not production Turso)?
2. Are smoke test mailboxes accumulating in the smoke server's local DB without cleanup?
3. Should the smoke server have its own Turso instance, or is local SQLite sufficient?
4. Is the `db-check.yml` workflow showing smoke DB contents vs production DB contents correctly?

## Related files

- Deploy smoke: `.github/workflows/deploy-smoke.yml` (no Turso env vars)
- Deploy production: `.github/workflows/deploy-production.yml` (has Turso env vars)
- Smoke test: `.github/workflows/smoke-test-periodic.yml`
- Database init: `cmd/app/main.go:31-46` (dual DB logic)
- Config: `internal/platform/config/config.go`
