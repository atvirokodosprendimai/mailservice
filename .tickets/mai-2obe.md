---
id: mai-2obe
status: closed
deps: []
links: [mai-1y8l, mai-3k6g, mai-bpn7, mai-koa7]
created: 2026-03-08T11:45:17Z
type: task
priority: 2
assignee: ~.~
tags: [payments, backend, polar]
---
# Add provider-neutral payment seam for key mailbox flow

Introduce or tighten the payment port used by the key-bound mailbox claim flow so checkout/session creation and payment completion are not hard-wired to legacy provider-specific naming.

## Acceptance Criteria

Core service uses a provider-neutral payment seam for new key-bound mailbox checkout
Key-bound claim flow can request a payment session without depending on legacy Stripe-specific types
Existing legacy payment flow keeps working during the change

