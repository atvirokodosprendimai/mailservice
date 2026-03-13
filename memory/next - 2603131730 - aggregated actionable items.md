---
tldr: Aggregated actionable items after smoke test headless browser fix shipped
---

# Next — 2026-03-13 17:30

1 - Gift Coupon E2E Verification (Phase 5) — partially verified
  - 1.1 - [x] Run smoke test with coupon code against sandbox instance (CI green, 6/6)
  - 1.2 - [x] Verify: claim with "OPENCLAWS" → $0 Polar checkout → webhook → mailbox active
  - 1.3 - [ ] Verify: claim without coupon → normal payment flow, ExpiresAt = +1 month
  - 1.4 - [ ] Verify: Polar dashboard shows $0 order with discount applied
  - 1.5 - [ ] Verify: after 23 uses, next claim with coupon returns 410
  - 1.6 - [ ] Mark gift coupon plan Phase 5 items complete

2 - Stale Plan (shipped, needs status update)
  - 2.1 - [[plan - 2603111027 - edproof challenge-response to verify private key ownership]] — mark status: completed

3 - Open Plan
  - 3.1 - [[docs/plans/2026-03-11-003-feat-agent-api-skill-for-mailservice-plan.md]] — agent API skill, untested

4 - Todos
  - 4.1 - [[todo - 2603101246 - payment confirmation page should use same css as main page]]
  - 4.2 - [[todo - 2603110114 - add data integrity check for known mailbox IDs]]
  - 4.3 - [[todo - 2603110814 - setup database backups]]
  - 4.4 - [[todo - 2603110829 - proactive mailbox expiry instead of lazy check on access]]
