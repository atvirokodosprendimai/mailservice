---
tldr: DATABASE_MODE env var required — turso or local, no silent fallback
category: ops
---

# Solved: Explicit database mode prevents silent fallback

Silent fallbacks are an antipattern.
The current code (`cmd/app/main.go:38-46`) silently falls back to local SQLite when `TURSO_DATABASE_URL` is empty.
A misconfigured production deploy would silently write to a local file instead of failing.

## The fix

Add a `DATABASE_MODE` env var (or similar) that explicitly declares intent:
- `DATABASE_MODE=turso` — requires `TURSO_DATABASE_URL` + `TURSO_AUTH_TOKEN`, fails if missing
- `DATABASE_MODE=local` — uses local SQLite at `DATABASE_DSN`
- No default — app refuses to start without explicit selection

Smoke server sets `DATABASE_MODE=local`.
Production sets `DATABASE_MODE=turso`.

## Also

- Smoke test mailboxes accumulate in the smoke server's local DB without cleanup — add a TTL or cleanup job
- `db-check.yml` should verify which database mode each environment is using

## Related files

- Database init: `cmd/app/main.go:31-46`
- Config: `internal/platform/config/config.go`
- Deploy smoke: `.github/workflows/deploy-smoke.yml`
- Deploy production: `.github/workflows/deploy-production.yml`
