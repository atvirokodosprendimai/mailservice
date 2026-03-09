---
id: mai-m6db
status: open
deps: []
links: []
created: 2026-03-09T12:21:46Z
type: task
priority: 1
assignee: ~.~
tags: [email, integrations, unsend]
---
# Send transactional mail via Unsend hosted instance

Integrate transactional email delivery through the hosted Unsend instance at https://unsend.admin.lt/. Wire runtime configuration to use repository secret UNSEND_KEY for API authentication.

## Acceptance Criteria

- Add Unsend notifier adapter and configuration wiring\n- Use repository secret/env var UNSEND_KEY for authentication\n- Route transactional emails (payment/recovery/notifications) through Unsend when configured\n- Keep existing notifier fallback behavior intact\n- Add tests for configured and unconfigured paths\n- Document required env vars in README/.env.example

