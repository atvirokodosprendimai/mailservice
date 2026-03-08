---
id: mai-zhom
status: open
deps: []
links: []
created: 2026-03-08T12:12:08Z
type: task
priority: 2
assignee: ~.~
tags: [ci, cicd, developer-experience, opentofu]
---
# Add local pre-push validation for GitHub Actions and OpenTofu

Add a lightweight local validation path for workflow changes so formatting and validate failures are caught before push. Cover the Hetzner OpenTofu workflow first.

## Acceptance Criteria

Repo documents the local validation commands for workflow changes
OpenTofu workflow changes have a local pre-push checklist including fmt and validate
Optional local GitHub Actions execution path using act is documented
At least one ticket or follow-up note identifies how developers should use the validation path before pushing infra workflow changes

