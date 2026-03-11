---
tldr: The db-check workflow reports what exists but never validates what should exist
---

# Incoherence: DB check workflow counts rows but never checks expected records

The `db-check.yml` workflow reports:
- Row counts per table
- Mailboxes by status
- All mailbox rows

It never asks: "are the records that should be here actually here?"

A workflow that only describes current state without comparing to expected state is a reporting tool, not a health check.
