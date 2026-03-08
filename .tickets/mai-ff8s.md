---
id: mai-ff8s
status: open
deps: [mai-3uex]
links: []
created: 2026-03-08T09:50:58Z
type: task
priority: 3
assignee: ~.~
parent: mai-sehx
tags: [cleanup, migration, backend]
---
# Remove legacy account-centric auth after client migration

After key-bound clients have migrated, remove account-centric mailbox auth paths such as API-token mailbox access, refresh-token auth for mailbox use, and recovery-based mailbox access assumptions. This should only happen after compatibility is no longer required.

## Acceptance Criteria

Removal plan identifies which legacy endpoints and tables can be retired\nMailbox access no longer depends on account API tokens after migration\nCleanup work is deferred until migration exit criteria are met

