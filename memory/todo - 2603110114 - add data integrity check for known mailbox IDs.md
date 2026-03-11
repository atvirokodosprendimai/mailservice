---
tldr: Validate that expected mailbox records exist — not just row counts
category: ops
---

# Todo: Add data integrity check for known mailbox IDs

The db-check workflow only counts rows and lists what's there.
It never checks whether specific expected records are present.

Add a check that:
- Maintains a list of known active mailbox IDs (could be a file in the repo or a Turso query)
- On each deploy or periodically, verifies all known IDs still exist
- Fails loudly if any are missing

This would have caught the `180d1d9c` loss immediately after the server rebuild.
