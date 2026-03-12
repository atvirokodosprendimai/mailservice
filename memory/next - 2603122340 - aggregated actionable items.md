---
tldr: Aggregated actionable items after gift coupon feature shipped
---

# Next — 2026-03-12 23:40

1 - Gift Coupon E2E Verification (Phase 5)
  - 1.1 - Run smoke test with coupon code against sandbox instance
  - 1.2 - Verify: claim with "OPENCLAWS" → $0 Polar checkout → webhook → mailbox active, ExpiresAt = +3 months
  - 1.3 - Verify: claim without coupon → normal payment flow unchanged
  - 1.4 - Verify: Polar dashboard shows $0 order with discount applied

2 - Open Todos
  - 2.1 - [[todo - 2603101246 - payment confirmation page should use same css as main page]]
  - 2.2 - [[todo - 2603110114 - add data integrity check for known mailbox IDs]]
  - 2.3 - [[todo - 2603110814 - setup database backups]]
  - 2.4 - [[todo - 2603110829 - proactive mailbox expiry instead of lazy check on access]]

3 - Stale Plan (already shipped, needs cleanup)
  - 3.1 - [[plan - 2603111027 - edproof challenge-response to verify private key ownership]] — mark completed
