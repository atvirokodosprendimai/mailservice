---
id: mai-741t
status: open
deps: []
links: [mai-sehx]
created: 2026-03-08T09:52:42Z
type: task
priority: 2
assignee: ~.~
tags: [infra, cicd, github-actions, hetzner, opentofu]
---
# Set up GitHub Actions CI/CD to Hetzner with OpenTofu

Design and implement CI/CD for mailservice using GitHub Actions and OpenTofu to provision and deploy to Hetzner. The workflow should cover infrastructure state management, environment configuration, deployment sequencing, and safe promotion of application changes to the target Hetzner environment.

## Acceptance Criteria

Repository contains a documented CI/CD workflow for GitHub Actions\nInfrastructure provisioning and update steps use OpenTofu rather than Terraform\nDeployment target and required Hetzner resources are defined clearly\nSecrets and state handling requirements are documented\nRollout and rollback expectations are defined before implementation starts

