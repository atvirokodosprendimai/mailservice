---
tldr: Configure Dovecot to listen on port 993 with TLS so IMAPS connections work
category: infra
---

# Todo: enable IMAPS port 993 with TLS in Dovecot

Port 993 is open in both Hetzner firewall (OpenTofu) and NixOS firewall (`mailservice-gitops.nix`), but Dovecot isn't serving TLS on it.

## What needs doing

- Provision a TLS certificate for `mail.truevipaccess.com` (Let's Encrypt via ACME, or self-signed for internal use)
- Configure Dovecot `ssl = required` (or `ssl = yes`) with cert/key paths in `nix/modules/mailservice-gitops.nix`
- Verify `nc -z mail.truevipaccess.com 993` succeeds after deploy

## Related files

- `nix/modules/mailservice-gitops.nix` — Dovecot config, currently `enableImap = true` but no SSL settings
- `infra/opentofu/main.tf` — firewall rule for port 993 already present

## Context

Port 143 (plaintext IMAP / STARTTLS) is working.
Port 25 (SMTP) is working.
Port 993 (IMAPS) — firewall open, Dovecot not listening.
