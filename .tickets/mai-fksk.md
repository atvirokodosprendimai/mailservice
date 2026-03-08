---
id: mai-fksk
status: closed
deps: []
links: [mai-741t]
created: 2026-03-08T10:51:23Z
type: task
priority: 2
assignee: ~.~
tags: [infra, cicd, github-actions, opentofu, prod]
---
# Add CI/CD plan gate before production deploy

Require a dedicated OpenTofu plan stage in GitHub Actions before any production apply/deploy step. The workflow should surface the plan output for review and prevent automatic production rollout without the plan gate passing.

## Acceptance Criteria

GitHub Actions workflow has a separate OpenTofu plan stage before production deployment
Production apply/deploy depends on successful plan stage completion
Plan output is persisted or surfaced for operator review
Production deployment is not triggered directly from code changes without the plan gate

