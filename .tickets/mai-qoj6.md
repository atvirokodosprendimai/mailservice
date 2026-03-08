---
id: mai-qoj6
status: closed
deps: []
links: []
created: 2026-03-08T09:50:58Z
type: task
priority: 3
assignee: ~.~
parent: mai-sehx
tags: [design, pop3, httpapi, future]
---
# Design POP3 and future HTTP read API on key-bound auth

Document the protocol additions that can follow the initial redesign: POP3 read access and a future read-only HTTP message API. Both must reuse the same edproof key-binding and active-subscription policy model.

## Acceptance Criteria

Design notes cover POP3 resolve requirements\nDesign notes cover read-only HTTP message API endpoints\nBoth designs explicitly reuse key-bound auth and inbound-only scope\nNo SMTP or outbound capabilities are introduced

