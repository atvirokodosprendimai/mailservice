---
id: mai-4vll
status: closed
deps: []
links: []
created: 2026-03-08T12:32:12Z
type: task
priority: 2
assignee: ~.~
tags: [deploy, cicd, github-actions, hetzner, production]
---
# Complete GitHub Actions app deployment on Hetzner host

Extend the current Hetzner/OpenTofu pipeline so it not only plans/applies infra but also deploys the application runtime on the target host. Cover env delivery, host-side compose or service restart, health checks, and rollback expectations.

## Acceptance Criteria

GitHub Actions workflow describes or implements deployment of the app runtime after infra apply
Required production env and secret delivery to the host is defined
Post-deploy health check is included in the deployment path
Rollback or failure handling is documented for app deployment failures

