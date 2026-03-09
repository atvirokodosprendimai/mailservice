---
id: mai-mtul
status: open
deps: []
links: []
created: 2026-03-09T08:36:20Z
type: bug
priority: 1
assignee: ~.~
tags: [api, errors, dx]
---
# Return actionable error when verifier backend is unavailable

Current error 'key proof verifier not configured' is too internal and leaves agents stuck. Need stable actionable error contract for clients when verifier dependency is missing/unhealthy.

## Acceptance Criteria

- Map verifier-unavailable to clear API error message\n- Keep error payload contract stable ({"error":"..."})\n- Document retry/support guidance for clients\n- Add handler test for this failure mapping

