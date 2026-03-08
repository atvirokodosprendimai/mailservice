---
id: mai-whp4
status: open
deps: [mai-ec6j]
links: []
created: 2026-03-08T09:50:44Z
type: feature
priority: 1
assignee: ~.~
parent: mai-sehx
tags: [backend, httpapi, access]
---
# Implement POST /v1/access/resolve for key-based inbound access

Add key-based access resolution for inbound mailbox protocols. The endpoint must authenticate with edproof, locate the mailbox by key fingerprint, enforce active subscription status, and return inbound access details only.

## Acceptance Criteria

Valid proof for active mailbox returns IMAP access details\nValid proof for unpaid or expired mailbox is denied\nResolve response contains inbound access fields only and no SMTP submission data\nTests cover wrong-key denial and expired subscription denial

