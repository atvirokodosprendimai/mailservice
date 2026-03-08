# Deployment View

## Current Production Shape

The current target is a single Hetzner host fronted by Cloudflare Tunnel for `truevipaccess.com`.

## Nodes

| Node | Role |
| --- | --- |
| GitHub Actions | Runs validation, OpenTofu plan/apply, and host deployment steps. |
| Hetzner Cloud VM | Main runtime host for the API and receive-only mail service. |
| Cloudflare Tunnel | Public HTTP ingress to the API without directly exposing the API port. |
| Polar | External payment system used during mailbox claim and payment confirmation. |

## Runtime Services On The Host

| Service | Purpose |
| --- | --- |
| `mailservice` | Main API container. |
| `mailreceive` | Receive-only mail stack for inbound mail and IMAP access. |
| `cloudflared` | Temporary reverse-proxy ingress for `truevipaccess.com`. |
| SQLite volume | Shared persistent state for API and mail runtime. |

## Deployment Flow

1. GitHub Actions validates the workflow and OpenTofu configuration.
2. GitHub Actions runs OpenTofu plan.
3. Production apply is gated behind the plan stage and environment approval.
4. After infrastructure apply, GitHub Actions uploads compose/runtime files to the host.
5. The host runs `docker compose pull` and `docker compose up -d`.
6. GitHub Actions performs a health check against `PUBLIC_BASE_URL`.

## Operational Constraints

- Cloudflare Tunnel is the current temporary ingress path, not the final permanent edge design.
- The app must keep `PUBLIC_BASE_URL` aligned with the public hostname so payment return URLs remain valid.
- The deployment docs and workflow should stay aligned with `compose.tunnel.yml.example`.
