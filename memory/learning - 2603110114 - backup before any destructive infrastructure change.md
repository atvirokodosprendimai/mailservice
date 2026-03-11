---
tldr: Always export and verify data before server rebuilds or database migrations
---

# Learning: Backup before any destructive infrastructure change

Before any server rebuild, database migration, or infrastructure change:
1. Export the full database to a known-good location (not on the server being rebuilt)
2. Verify the export contains all expected records — not just row counts, but known IDs
3. After the change, diff the new state against the export

This applies to:
- NixOS rebuilds / reprovisioning
- Database migrations (local SQLite to Turso, or any other)
- Server replacement / scaling
