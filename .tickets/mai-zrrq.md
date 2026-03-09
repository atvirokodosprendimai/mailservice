---
id: mai-zrrq
status: open
deps: []
links: []
created: 2026-03-09T08:36:20Z
type: bug
priority: 0
assignee: ~.~
tags: [api, identity, onboarding]
---
# Claim mailbox fails when key proof verifier is unset

During real onboarding, claiming a mailbox failed with error: 'key proof verifier not configured'. This blocks first-time setup after generating Ed25519 key and providing billing email. API appears up but cannot validate key proof.

## Acceptance Criteria

- Reproduce failure in current environment\n- Configure/initialize key proof verifier in production deploy path\n- POST /v1/mailboxes/claim accepts valid edproof and returns expected pending/payment response\n- Add regression test for missing verifier config path

