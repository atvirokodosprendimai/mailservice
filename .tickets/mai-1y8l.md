---
id: mai-1y8l
status: closed
deps: [mai-2obe]
links: [mai-3k6g, mai-2obe, mai-bpn7, mai-koa7]
created: 2026-03-08T11:45:17Z
type: task
priority: 2
assignee: ~.~
tags: [payments, polar, httpapi]
---
# Create Polar checkout for POST /v1/mailboxes/claim

Wire the key-bound mailbox claim endpoint to create a Polar payment session using POLAR_TOKEN and return/send the resulting payment link.

## Acceptance Criteria

POST /v1/mailboxes/claim creates a Polar checkout for new or renew-needed key-bound mailboxes
Billing email receives or is associated with the Polar checkout link
Response includes the payment URL needed to complete checkout
Tests cover successful checkout creation and provider failure handling

