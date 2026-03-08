---
id: mai-w6k6
status: open
deps: [mai-whp4]
links: []
created: 2026-03-08T09:50:58Z
type: task
priority: 2
assignee: ~.~
parent: mai-sehx
tags: [migration, compatibility, backend]
---
# Preserve legacy account/token flow during key-based migration

Keep the existing account, refresh-token, and recovery-based mailbox flow operational while the new key-bound endpoints are introduced. Mark legacy behavior clearly in code and docs and avoid breaking current clients during rollout.

## Acceptance Criteria

Existing account/token endpoints continue to function during migration\nLegacy flow is identified clearly in docs and code comments where needed\nRegression tests cover unchanged legacy behavior for key mailbox operations still using old paths

