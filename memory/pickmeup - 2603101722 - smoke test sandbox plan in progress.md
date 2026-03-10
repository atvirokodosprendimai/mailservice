---
tldr: Pickmeup for 2026-03-08 to 2026-03-10 — sandbox smoke test infra built, script mid-edit
---

# Pickmeup: 2026-03-08 — 2026-03-10

## Timeline

### 2026-03-10 (Tuesday)

- `592fbf0` Add todo: periodic smoke test with Polar sandbox
- `2f01215` Add periodic smoke test — 5 checks every 5 minutes
- `1671f09` Update todo: document payment testing architecture constraint
- `74169fa` Add plan: sandbox smoke test instance with full payment flow
- `5530070` Revise plan: separate server and domain for smoke tests
- `f235343` Mark Phase 1.1 complete: sandbox product already exists
- `70f926e` Add subdomain and direct A record support to OpenTofu config
- `ad9d94b` Add NixOS host config for smoke test server
- `33acaac` Add deploy workflow for smoke test server
- `246b166` Update plan: Phase 2 infrastructure code complete
- => branch `task/periodic-smoke-test-sandbox` active (5 commits ahead of main)
- => branch `task/periodic-smoke-test` merged to main earlier

## Plans

### [[plan - 2603101317 - sandbox smoke test instance with full payment flow]]
- **Status:** active
- **Progress:**
  - Phase 1: 1/3 done (1.2 + 1.3 blocked on server existing)
  - Phase 2: 4/4 done (code complete, server not yet provisioned)
  - Phase 3: 0/3 done (script partially edited, uncommitted)
  - Phase 4: 0/4 done
- **Last action:** Phase 2.4 — deploy workflow created
- **Next action:** Phase 3.1 — finish updating smoke test script with auto-pay mode
- **Key discovery:** Stripe blocks API-based card tokenization from non-browser contexts. Switched to free sandbox product (`ce03e78f-930b-4693-93a0-6b0ff67aff7c`) — checkout confirms without Stripe, webhook still fires.

## Completed
- [[solved - 2603101244 - end to end production smoke test passes]]

## Still Open
- [[todo - 2603101248 - periodic smoke test using Polar sandbox for full claim to read cycle]]
- [[todo - 2603101246 - payment confirmation page should use same css as main page]]

## Uncommitted Work

- `ops/smoke-test-periodic.sh` has partial edits: header, usage, and arg parsing updated for `--auto-pay`, `--polar-token`, `--polar-api` flags. The auto-pay logic after claim (confirm checkout, poll for activation) is not yet written.

## Where You Left Off

Focus was building a fully isolated smoke test environment (separate Hetzner server, own domain, Polar sandbox).
Infrastructure code is done (OpenTofu, NixOS host, flake, deploy workflow) but the server isn't provisioned yet — needs `smoke` GitHub environment setup.
Mid-way through updating the smoke test script to auto-confirm free Polar checkouts.
Natural next step: finish the script's auto-pay logic, commit, then set up the GitHub environment and provision the server.
