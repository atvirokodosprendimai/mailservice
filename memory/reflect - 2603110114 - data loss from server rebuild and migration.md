---
tldr: Post-mortem on losing customer mailbox 180d1d9c — silent failures at every layer
---

# Reflect: Data loss from server rebuild and migration

## What happened

Customer mailbox `180d1d9c-0f4b-40bd-8cd5-d83ebc4b01a6` was lost.
The server was rebuilt from a NixOS snapshot that was already missing the record.
The Turso migration seeded from that incomplete data.
Nobody noticed.

## Extracted items

1 - Learnings
- 1.1 - Server rebuilds silently lose data without integrity checks
  - => [[learning - 2603110114 - server rebuilds silently lose data without integrity checks]]
- 1.2 - Silent fallbacks hide failures
  - => [[learning - 2603110114 - silent fallbacks hide failures]]
- 1.3 - Seeding from a damaged source propagates the damage
  - => [[learning - 2603110114 - seeding from a damaged source propagates the damage]]
- 1.4 - Backup before any destructive infrastructure change
  - => [[learning - 2603110114 - backup before any destructive infrastructure change]]

2 - Tasks
- 2.1 - Add data integrity check for known mailbox IDs
  - => [[todo - 2603110114 - add data integrity check for known mailbox IDs]]
- 2.2 - Contact the customer whose mailbox was lost
  - => [[todo - 2603110114 - contact customer whose mailbox was lost]]

3 - Incoherences
- 3.1 - Smoke test plan marked solved while polluting production database
  - => [[incoherence - 2603110114 - smoke test plan marked solved while polluting production database]]
- 3.2 - DB check workflow counts rows but never checks expected records
  - => [[incoherence - 2603110114 - db check workflow counts rows but never checks expected records]]
