---
id: mai-4my9
status: closed
deps: [mai-ic8q]
links: []
created: 2026-03-08T20:00:00Z
type: task
priority: 2
assignee: ~.~
tags: [infra, deployment, nixops, nixos, hetzner]
---
# Migrate the NixOS deployment path to NixOps

Move the repo from a NixOS target model to a concrete NixOps-managed deployment
path for the single Hetzner VM. This includes the NixOps network definition,
operator commands, placeholder guards, and the migration/cutover documentation
needed to replace the current Ubuntu host with a NixOS host.

## Acceptance Criteria

The repo contains a NixOps network definition for `truevipaccess`
The repo contains create, deploy, and rollback command wrappers
Placeholder image refs fail clearly instead of silently deploying
A migration plan documents the cutover from Ubuntu to a fresh NixOS host
The repo exposes the NixOps path as the preferred future deployment model
