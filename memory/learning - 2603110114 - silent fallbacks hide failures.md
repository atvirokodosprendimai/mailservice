---
tldr: The app silently fell back to local SQLite when Turso wasn't configured — hiding that production data was being written to an ephemeral disk
---

# Learning: Silent fallbacks hide failures

When `TURSO_DATABASE_URL` was empty, the app silently used local SQLite.
This meant:
- A misconfigured deploy writes to a file that dies with the server
- No log line distinguishes "intentionally local" from "accidentally local"
- The smoke server was writing to local SQLite for weeks without anyone realising it wasn't durable

Silent fallbacks are an antipattern.
Explicit configuration that fails loudly on misconfiguration is always better.
