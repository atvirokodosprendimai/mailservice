---
id: mai-6vu0
status: closed
deps: []
links: []
created: 2026-03-08T12:30:58Z
type: task
priority: 2
assignee: ~.~
tags: [payments, polar, webhooks, production]
---
# Finish Polar payment integration hardening

Complete the remaining production-grade Polar work beyond the minimal slice. Focus on signed webhook verification, callback hardening, provider assumptions, and operational cleanup needed to treat Polar as the normal payment path.

## Acceptance Criteria

Signed Polar webhook verification replaces or complements the temporary success-callback path
Payment completion handling is safe against replay or tampering assumptions left in the minimal slice
Docs describe the intended production Polar flow clearly
Any remaining provider-specific cleanup needed for normal Polar operation is identified or completed

## Resolution

Implemented signed `POST /v1/webhooks/polar` verification using the Standard Webhooks header scheme, kept the success callback as fallback, and updated deploy/runtime docs and templates to include `POLAR_WEBHOOK_SECRET`.
