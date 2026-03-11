---
tldr: Pickmeup for 2026-03-09 to 2026-03-11
---

# Pickmeup: 2026-03-09 — 2026-03-11

## Timeline

### 2026-03-11 (Wednesday)

- `963a47c` Reflect on data loss: 4 learnings, 2 todos, 2 incoherences
  - [reflect - 2603110114 - data loss from server rebuild and migration.md](<memory/reflect - 2603110114 - data loss from server rebuild and migration.md>)
- `81a21ce` Add Turso database config to smoke deploy
  - [deploy-smoke.yml](<.github/workflows/deploy-smoke.yml>)
- `f53fbc7` Refine todo: silent DB fallback is antipattern
  - [todo - 2603110050 - investigate why smoke tests are not using smoke test database.md](<memory/todo - 2603110050 - investigate why smoke tests are not using smoke test database.md>)
- `8e665a0` Add todo: investigate smoke test database isolation
- `9f5bc8d` Complete smoke test plan: mark todo solved, finalize Phase 4
- `65ba90b` Fix Turso seed: use TURSO_API_TOKEN for CLI auth
- `48714dd` Fix Turso seed: use turso CLI shell for data import
- `89095f3` Fix Turso seed: import full schema + data via HTTP pipeline
- `86ece36` Fix Turso seed: use HTTP pipeline API for data import
- `2bb585c` Add result symlink to gitignore
- => branches: `task/reflect-data-loss`, `task/smoke-turso-db`, `task/fix-silent-db-fallback`, `task/fix-turso-seed` (v1-v4), `task/complete-smoke-test-plan`, `task/smoke-db-investigation-todo`

## Plans

### [[plan - 2603101317 - sandbox smoke test instance with full payment flow]]
- **Status:** completed
- **Progress:** All 4 phases done
- **Last action:** Phase 4.4 — retained persistent-key mode as alternative, workflow uses auto-pay only
- **Result:** Full E2E smoke test runs every 5 min on isolated server — healthz → claim → pay → activate → resolve → IMAP → messages

## Completed
- [[solved - 2603110049 - periodic smoke test using Polar sandbox for full claim to read cycle]]
- [[solved - 2603101244 - end to end production smoke test passes]]

## Still Open
- [[todo - 2603110114 - contact customer whose mailbox was lost]]
- [[todo - 2603110114 - add data integrity check for known mailbox IDs]]
- [[todo - 2603110050 - investigate why smoke tests are not using smoke test database]]
- [[todo - 2603101246 - payment confirmation page should use same css as main page]]

## Incoherences Flagged
- [[incoherence - 2603110114 - smoke test plan marked solved while polluting production database]]
- [[incoherence - 2603110114 - db check workflow counts rows but never checks expected records]]

## Where You Left Off

The big push was finishing the sandbox smoke test plan — a fully isolated smoke server with automated Polar checkout running every 5 minutes.
That's done and green in CI.

Then you reflected on the data loss incident (customer mailbox `180d1d9c` lost during server rebuild).
The reflection surfaced 4 learnings and 2 incoherences — notably that smoke tests are hitting the production database, not the smoke DB.
The most urgent open item is contacting the affected customer; the most impactful engineering item is fixing the DB isolation issue.
