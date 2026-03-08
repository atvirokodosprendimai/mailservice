---
id: mai-koa7
status: closed
deps: [mai-3k6g]
links: [mai-1y8l, mai-3k6g, mai-2obe, mai-bpn7]
created: 2026-03-08T11:45:17Z
type: task
priority: 3
assignee: ~.~
tags: [docs, payments, polar]
---
# Document minimal Polar payment flow for key-bound mailboxes

Update docs to describe the minimal production path: claim mailbox, pay via Polar, and resolve IMAP access with the same key.

## Acceptance Criteria

README or follow-up docs describe the Polar-backed claim-pay-resolve flow
Required secrets include POLAR_TOKEN where relevant
Legacy Stripe wording is not presented as the preferred new flow

