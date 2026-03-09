---
id: mai-wvcw
status: open
deps: []
links: []
created: 2026-03-09T12:20:19Z
type: task
priority: 1
assignee: ~.~
tags: [email, integration, ops]
---
# Send transactional mail via hosted Unsend instance

Integrate transactional email delivery through hosted Unsend at https://unsend.admin.lt/. Use repository secret UNSEND_KEY for API authentication in runtime/deploy configuration.

## Acceptance Criteria

- Add Unsend notifier adapter and wire it into notifier selection\n- Read API key from UNSEND_KEY secret/environment variable\n- Route transactional/recovery/payment emails through Unsend when configured\n- Add tests for notifier wiring and request payload behavior\n- Document Unsend setup in README/.env.example and deployment notes

