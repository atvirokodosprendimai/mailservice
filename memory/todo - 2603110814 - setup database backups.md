---
tldr: Automated backups for production Turso database and local SQLite
category: ops
---

# Todo: Setup database backups

After the data loss incident ([[reflect - 2603110114 - data loss from server rebuild and migration]]), we need automated backups.

- Production Turso DB — check if Turso has built-in backup/export, or set up periodic `turso db shell` dump
- Local SQLite on servers — periodic copy to offsite storage (S3, Hetzner storage box, etc.)
- Verify restorability — backups that aren't tested are not backups
- Related: [[learning - 2603110114 - backup before any destructive infrastructure change]]
