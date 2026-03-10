---
tldr: Pickmeup for 2026-03-07 to 2026-03-10
---

# Pickmeup: 2026-03-07 — 2026-03-10

## Timeline

### 2026-03-09 (Monday)

**Session 1 — pricing, Polar, DNS, IMAP delivery pipeline**
- `126b8ce` Mark all implemented spec tasks as done
- `36f14b8` Open ports 80 and 443 in NixOS firewall for ACME HTTP-01 validation
- `addf7cd` Reload Dovecot when ACME cert renews
- `24e12e1` Trigger ACME renewal on deploy when cert is missing or self-signed
- `bee2459` Fix ACME deploy step: add sudo, handle corrupt certs
- `4d20501` Fix ACME renewal: reset-failed + restart instead of start
- `d3d5c30` Add ACME renewal diagnostics: journal on failure, cert check
- `48324ed` Add detailed ACME diagnostics: directory listing, journal
- `c8467cd` Fix Dovecot ACME cert: add nginx group, fix deploy cert check permissions
- `0263829` Add session export — drift reconciliation and ACME TLS fix
  - [deploy-production.yml](<.github/workflows/deploy-production.yml>)
  - [mailservice-gitops.nix](<nix/modules/mailservice-gitops.nix>)

**Branches active:**
- `task/acme-debug`, `task/acme-dovecot-reload`, `task/acme-restart-fix`
- `task/fix-dovecot-acme-group`, `task/session-export-acme`
- `task/next-snapshot`

### 2026-03-07 — 2026-03-08

No commits.

## Sessions

- [[session - 2603091637 - pricing copy polar DNS IMAP delivery pipeline]]
  — Website copy, Polar product, Cloudflare DNS, Postfix SQLite fix, CI/CD secrets
- [[session - 2603092350 - drift reconciliation and ACME TLS fix]]
  — Marked all spec tasks done, fixed layered ACME cert renewal issues (6 root causes, 7 PRs)

## Learnings Captured

- [[learning - 2603091640 - Postfix temporary lookup failure means unsupported map type]]
- [[learning - 2603091640 - NixOS postfix has no package option use overlays]]
- [[learning - 2603091640 - NixOS Postfix needs explicit SQLite support via overlay]]
- [[learning - 2603091640 - NixOS Postfix config lives in var lib postfix conf]]
- [[learning - 2603091640 - Cloudflare Terraform provider 4.0 uses v3-era resource names]]

## Completed

- [[solved - 2603091556 - enable IMAPS port 993 with TLS in Dovecot]]
- [[solved - 2603091559 - show IMAP connection instructions on the website]]
- [[drift - 2603092102 - specs fully implemented but tasks unchecked]] — resolved, all tasks marked done

## Still Open

### Specs with all tasks done (no remaining implementation work)
- Key-bound mailbox spec — 7/7 tasks done
- Polar minimal payments spec — 13/13 tasks done
- Unsend transactional mail spec — 5/5 tasks done

### Minor
- Deploy ACME check triggers unnecessarily (cosmetic — lego just says "no renewal needed")

### Overdue Reminder
- Cargo cult reading list (due 2026-03-05)

## Where You Left Off

The last two sessions were a big push: website copy, Polar payments, DNS, and fixing mail delivery (Postfix SQLite).
Then a deep ACME debugging session fixed TLS certificates for IMAPS — `mail.truevipaccess.com:993` now serves a valid Let's Encrypt cert.
All three feature specs (key-bound mailbox, Polar payments, Unsend transactional mail) are fully implemented with tasks marked done.
The natural next step is either shipping/testing the live service end-to-end, or tackling new feature work.
