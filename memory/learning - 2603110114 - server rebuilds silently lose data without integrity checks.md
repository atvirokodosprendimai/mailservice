---
tldr: Rebuilding from a NixOS snapshot lost a customer mailbox — no check caught it
---

# Learning: Server rebuilds silently lose data without integrity checks

A server rebuild from a NixOS snapshot silently dropped mailbox `180d1d9c`.
Nobody noticed because:
- The app started fine with whatever data the snapshot had
- The db-check workflow only counted rows, never checked for specific expected records
- The Turso seed propagated the already-incomplete data to the cloud database

The failure was invisible until the customer was gone.
