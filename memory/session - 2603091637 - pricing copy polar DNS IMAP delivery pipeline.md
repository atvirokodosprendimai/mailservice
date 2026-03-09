---
tldr: End-to-end deployment session — website copy, Polar integration, DNS records, IMAP delivery fix, CI/CD secrets pipeline
---

# Session: pricing copy, Polar, DNS, IMAP delivery, pipeline

## What was accomplished

### Website & Pricing
- Created website copy with pricing (1 EUR/month) and IMAP connection instructions
- Added "Reading mail" section with two options: HTTP API (agent-first) and direct IMAP (human-secondary)
- Documents `/v1/imap/messages` and `/v1/imap/messages/get` endpoints

### Polar Payment Gateway
- Created Polar product with EUR + USD pricing via API
- POLAR_PRICE_ID = `01f68f36-4b6f-402a-b670-c7ebde03a836`

### CI/CD Secrets Pipeline
- Rewrote deploy workflow to inject ALL secrets from GitHub Secrets/Variables into production env file
- Added deploy-time validation of required env vars
- Configured all GitHub secrets and variables via `gh` CLI

### DNS & Infrastructure
- Added Cloudflare provider to OpenTofu
- Created `mail.truevipaccess.com` A record pointing to Hetzner server
- Created MX record for `truevipaccess.com` → `mail.truevipaccess.com`
- Added OpenTofu auto-apply on merge to main
- Opened ports 143, 993 in both Hetzner firewall and NixOS firewall

### Mail Delivery Fix (critical)
- **Root cause:** NixOS Postfix not compiled with SQLite support
- All `sqlite:` virtual mailbox lookups failed with "451 Temporary lookup failure"
- **Fix:** `nixpkgs.overlays` with `postfix.override { withSQLite = true; }`
- Also fixed Postfix SQLite config path (`/var/lib/postfix/conf/` not `/etc/postfix/`)

## Debugging notes

- Cloudflare provider `~> 4.0` uses v3-style resource names (`cloudflare_record`, `cloudflare_zones`), NOT v5 (`cloudflare_dns_record`, `cloudflare_zone`)
- `CLOUDFLARE_API_TOKEN` secret needed re-setting (was empty on first run)
- "Temporary lookup failure" = Postfix can't handle the map type, not just file-not-found
- NixOS `services.postfix.package` option doesn't exist — must use `nixpkgs.overlays`

## PRs merged
- #46 — Polar pricing, CI/CD secrets, Cloudflare DNS, OpenTofu auto-apply
- #47 — Cloudflare v4 syntax fix (then reverted in #48)
- #48 — Revert to v4.x compatible syntax
- #49 — IMAP docs on website + todos
- #50 — Enable SQLite in Postfix (wrong approach — `package` option)
- #51 — Fix: use overlay instead of package option

## Outstanding
- [[todo - 2603091556 - enable IMAPS port 993 with TLS in Dovecot]] — needs ACME cert + Dovecot SSL config
