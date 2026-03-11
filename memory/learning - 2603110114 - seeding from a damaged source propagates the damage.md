---
tldr: The Turso seed imported from post-rebuild SQLite — propagating the data loss to the new database
---

# Learning: Seeding from a damaged source propagates the damage

The Turso migration was seeded from the production server's local SQLite.
That SQLite was already missing mailbox `180d1d9c` after the server rebuild.
The seed faithfully copied the incomplete data to Turso.

The migration was declared successful based on row counts and table structure — not on whether all known records were present.
