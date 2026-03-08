---
id: mai-ec6j
status: closed
deps: []
links: []
created: 2026-03-08T09:50:44Z
type: feature
priority: 1
assignee: ~.~
parent: mai-sehx
tags: [backend, database, mailbox]
---
# Add key-bound mailbox schema and repository support

Extend mailbox persistence for the key-bound model. Add key_fingerprint and billing_email to mailbox storage, update repository mapping, and keep current provisioning behavior intact while new key-based flows are introduced.

## Acceptance Criteria

Mailbox persistence stores key_fingerprint uniquely\nMailbox persistence stores billing_email separately from any legacy owner field\nRepository read/write paths cover the new fields\nExisting mailbox provisioning tests remain green or are updated intentionally

