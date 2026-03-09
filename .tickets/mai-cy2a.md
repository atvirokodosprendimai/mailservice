# mai-cy2a Run API directly on NixOS under systemd, without Docker

Status: closed
Priority: P2

## Goal

Reduce Docker/Compose coupling in the NixOS deployment by running the API
service directly under NixOS systemd instead of pulling and starting the API
container image.

## Why

- current NixOS production still depends heavily on Docker image build/push
- API rollout is still coupled to GHCR availability and container runtime behavior
- the NixOS value proposition is weaker while the API remains a containerized app
- moving the API to a native NixOS service reduces deploy complexity and host drift

## Scope

- package or build the Go API in the Nix-based host configuration
- run the API under systemd on NixOS
- move API runtime env handling to the NixOS service model
- remove API container usage from the NixOS deployment path
- keep mailreceive/container runtime decisions separate for now

## Non-Goals

- removing Docker from the mailreceive stack in the same change
- redesigning the payment or mailbox flow
- changing public API contracts

## Acceptance Criteria

1. The NixOS host runs the API directly under systemd, not Docker Compose.
2. API runtime configuration is supplied through the NixOS service definition.
3. The NixOS deployment path no longer requires an `API_IMAGE` for the API service.
4. `https://truevipaccess.com/` and `/healthz` continue to work after the change.
5. The migration path is documented clearly enough for production rollout.
