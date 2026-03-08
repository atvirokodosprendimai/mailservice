---
id: mai-nf2m
status: closed
deps: []
links: []
created: 2026-03-08T18:05:00Z
type: task
priority: 1
assignee: ~.~
tags: [website, product, launch, http]
---
# Add production homepage for truevipaccess.com

Serve a real homepage at `/` so the public product entrypoint explains what the service is and how to start instead of returning `404`.

## Acceptance Criteria

`GET /` returns `200`
Homepage explains the key-bound inbound mailbox model in plain terms
Homepage states inbound-only scope and no SMTP sending
Homepage includes the first concrete action for agents or operators
Homepage does not break existing API endpoints
