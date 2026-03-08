# NixOps Migration Spec

## Overview

Move the repo from "NixOS target model only" to a concrete NixOps-managed
deployment path for the single Hetzner VM.

The goal is not to switch the live Ubuntu host in place. The goal is to make
the NixOS deployment reproducible, revision-driven, and operationally concrete
enough that a fresh NixOS host can replace the current box with a controlled
cutover.

## Requirements

### User Story

As the operator, I want this repo to define the NixOps deployment shape for the
`truevipaccess.com` host, so that the NixOS migration is executable rather than
just aspirational.

### Acceptance Criteria

1. WHEN an operator wants to create a NixOps deployment THEN the repo SHALL
   provide a NixOps network definition for the single `truevipaccess` host.
2. WHEN an operator deploys the NixOS host THEN the repo SHALL provide concrete
   create, deploy, and rollback commands.
3. IF pinned application images are still placeholders THEN the NixOS
   configuration SHALL fail with a clear assertion instead of deploying
   incomplete configuration.
4. WHEN an operator reads the migration docs THEN the repo SHALL define the
   exact secret files and cutover sequence required to replace the Ubuntu host.
5. WHEN the NixOps path is used THEN the repo SHALL preserve the existing rule
   that application artifacts are pinned in Git rather than pulled via mutable
   assumptions.

## Design

### Decision: NixOps Scope

Context:
- production is currently an Ubuntu VM with an imperative deploy path
- the repo already contains a NixOS host configuration
- the user wants NixOps specifically, not Kubernetes GitOps

Options considered:
1. Keep only `nixos-rebuild` docs
   - Pros: simpler
   - Cons: still no concrete deployment orchestration
2. Add NixOps management for a single existing/fresh NixOS VM
   - Pros: concrete workflow, rollback semantics, minimal scope change
   - Cons: still requires a fresh NixOS host and operational cutover

Decision:
- add NixOps management for the single host

Rationale:
- this turns the repo into an executable migration plan
- it avoids mixing Kubernetes or broader infra redesign into the current launch

### Components

- `nixops/default.nix`
  Defines the single-host NixOps network and target host details.
- `ops/nixops/create.sh`
  Creates the NixOps deployment state if it does not already exist.
- `ops/nixops/deploy.sh`
  Creates the deployment if needed, then runs `nixops deploy`.
- `ops/nixops/rollback.sh`
  Performs a `nixops rollback`.
- `flake.nix`
  Exposes `nixops-create`, `nixops-deploy`, and `nixops-rollback` app entries.
- `nix/modules/mailservice-gitops.nix`
  Adds assertions preventing placeholder image refs from being deployed.

### Secrets Contract

The host continues to expect:

- `/var/lib/secrets/mailservice.env`
- `/var/lib/secrets/cloudflared.env`

NixOps does not write those files in this initial migration step. They remain
operator-managed because they contain live secrets that should not be committed
or moved into deployment state casually.

## Testing Strategy

- `go test ./...` to verify the existing app code remains intact
- review the Nix files for placeholder assertion coverage
- review the migration plan for a full cutover sequence before use

## Out of Scope

- converting the current Ubuntu machine in place
- provisioning a Hetzner NixOS image automatically
- moving secrets into NixOps-managed secret material
- rewriting the existing GitHub Actions deployment path in the same change
