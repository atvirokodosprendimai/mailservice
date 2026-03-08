---
id: mai-3k6g
status: closed
deps: []
links: [mai-1y8l, mai-2obe, mai-bpn7, mai-koa7]
created: 2026-03-08T11:45:17Z
type: task
priority: 2
assignee: ~.~
tags: [payments, polar, webhook]
---
# Handle Polar payment completion for key-bound mailbox activation

Add the minimal Polar webhook or callback path needed to mark a key-bound mailbox paid and activate or renew it for one month.

## Acceptance Criteria

Polar payment completion can be correlated to a mailbox payment session
Successful payment activates or renews the key-bound mailbox
Invalid or duplicate callbacks are handled safely
Tests cover successful activation and rejected invalid callback cases

