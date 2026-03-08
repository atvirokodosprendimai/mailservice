---
id: mai-my77
status: closed
deps: [mai-mtk9]
links: []
created: 2026-03-08T09:50:58Z
type: task
priority: 2
assignee: ~.~
parent: mai-sehx
tags: [architecture, backend, mailbox]
---
# Refactor mailbox access services toward protocol-neutral core

Refactor core mailbox access APIs so policy is expressed in protocol-neutral service methods that can support IMAP today and POP3 or HTTP read APIs later. Keep protocol-specific behavior inside adapters.

## Acceptance Criteria

Core mailbox service methods are not hard-wired to IMAP-only naming where redesign work touches them\nProtocol-specific concerns remain in adapters\nRefactor does not broaden product scope beyond inbound read access

