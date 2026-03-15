---
name: next - 2603151720
description: Aggregated actionable items snapshot
type: next
---

1 - **Active Plan:** `docs/plans/2026-03-11-003-feat-agent-api-skill-for-mailservice-plan.md`
  - 1.1 - Test: agent can follow the skill end-to-end against a running instance
  - 1.2 - Walk through skill as agent against local instance
  - 1.3 - Verify every curl command works
  - 1.4 - Verify error messages match API responses

2 - **Todos**
  - 2.1 - Payment confirmation page should use same CSS as main page (frontend)
  - 2.2 - Add data integrity check for known mailbox IDs (ops)
  - 2.3 - Proactive mailbox expiry instead of lazy check on access (ops)

3 - **Just shipped (verify)**
  - 3.1 - Verify deploy succeeds with payment reconciliation + ADMIN_API_KEY
  - 3.2 - Confirm smoke test passes with reconciliation fallback
