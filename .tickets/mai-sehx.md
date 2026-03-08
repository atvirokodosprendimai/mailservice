---
id: mai-sehx
status: closed
deps: []
links: [mai-3uex, mai-jheu, mai-br0j, mai-741t]
created: 2026-03-08T09:47:08Z
type: epic
priority: 1
assignee: ~.~
tags: [product, architecture, edproof]
---
# Redesign mailservice around key-bound inbound mailboxes

Implement the key-bound inbound mailbox model described in docs/key-bound-mailbox-spec.md. Replace account-centric mailbox access with edproof-key-bound mailbox subscriptions, keep billing email as payment contact only, and preserve inbound-only product scope.

## Acceptance Criteria

- Spec accepted as implementation baseline
- Key-bound mailbox identity replaces account token as primary access model
- Inbound-only scope remains explicit across API and docs
