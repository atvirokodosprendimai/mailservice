---
id: mai-ic8q
status: closed
deps: []
links: []
created: 2026-03-08T17:46:30Z
type: task
priority: 2
assignee: ~.~
tags: [infra, deployment, nixos, gitops, hetzner, post-launch]
---
# Adopt NixOS GitOps deployment on the Hetzner VM

Replace the current Ubuntu plus imperative remote shell deployment path with a NixOS-based GitOps deployment model on the single Hetzner VM. The goal is to make host state declarative, remove package drift, and make deploy/rollback revision-based instead of shell-script-driven.

## Acceptance Criteria

Hetzner target can be provisioned or rebuilt as a NixOS host
Git is the source of truth for the VM runtime configuration
Application image refs are pinned in the NixOS deployment model rather than pulled ad hoc via mutable assumptions
Cloudflare tunnel and application runtime are represented in the NixOS configuration
Deploy and rollback are revision-based rather than imperative SSH shell steps
Migration plan from the current Ubuntu host is documented before cutover
