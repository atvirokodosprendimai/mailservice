---
tldr: The smoke test plan was marked complete while smoke data was accumulating in production Turso
---

# Incoherence: Smoke test plan marked solved while polluting production database

The smoke test todo was renamed to `solved` based on CI passing green.
Meanwhile, the old `smoke-production` job had already written 5 orphaned `pending_payment` records to the production Turso database.

"Green CI" is not the same as "correct system."
Marking work done based on CI status without checking the actual data state is a false signal.
