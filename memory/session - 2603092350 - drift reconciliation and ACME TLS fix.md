# Session — Drift reconciliation and ACME TLS fix

## Summary

Two main threads: reconciling spec task checkboxes with implemented code, and fixing ACME certificate renewal for `mail.truevipaccess.com` IMAPS.

## Drift Reconciliation

Ran `/eidos:next` then `/eidos:drift` across all three feature specs.
Found all specs fully implemented but every task checkbox still `[ ]`.

- **Key-bound mailbox spec** — 7 tasks marked done
- **Polar minimal payments spec** — 13 tasks marked done
- **Unsend transactional mail spec** — 5 tasks marked done

Drift level: low (tracking drift, not implementation drift).

## ACME TLS Fix

Started from a failed GitHub Actions deploy (run 22871787520).
The ACME cert renewal for `mail.truevipaccess.com` was failing.

### Root causes (layered)

1. **Port 80 closed in NixOS firewall** — only ports 25, 143, 993 were allowed.
   Let's Encrypt HTTP-01 challenge couldn't reach nginx.
   Fix: added ports 80 and 443 to `networking.firewall.allowedTCPPorts`.

2. **Dovecot not reloading on cert renewal** — NixOS ACME module wasn't configured to notify Dovecot.
   Fix: added `reloadServices = [ "dovecot2.service" ]` to ACME cert config.

3. **ACME timer not re-firing** — the service failed earlier and wouldn't retry until tomorrow.
   Fix: added deploy step to detect self-signed cert and trigger `systemctl start acme-*.service`.

4. **`systemctl start` no-op on failed oneshot** — needed `reset-failed` first.
   Fix: added `reset-failed` before `restart`.

5. **Cert file permissions** — ACME certs owned by `acme:nginx` (mode 640), but Dovecot only had `acme` supplementary group, not `nginx`.
   Dovecot couldn't read the Let's Encrypt cert and kept serving minica fallback.
   Fix: added `nginx` to Dovecot's supplementary groups.

6. **Deploy cert check permissions** — the `openssl` command in the deploy step ran without `sudo`, couldn't read the cert, and falsely triggered renewal every deploy.
   Fix: added `sudo` to the openssl call.

### PRs

- PR #58 — Open ports 80/443 in NixOS firewall + mark spec tasks done
- PR #59 — Reload Dovecot on ACME cert renewal
- PR #60 — Trigger ACME renewal on deploy when cert is self-signed
- PR #61 — Fix ACME renewal: reset-failed before restart
- PR #62 — Add ACME renewal diagnostics
- PR #63 — Add detailed ACME diagnostics (directory listing, journal)
- PR #64 — Fix Dovecot TLS: add nginx group for ACME cert access

### Result

`mail.truevipaccess.com:993` now serves a valid Let's Encrypt certificate:
```
issuer=C=US, O=Let's Encrypt, CN=E8
notBefore=Mar  9 19:39:13 2026 GMT
notAfter=Jun  7 19:39:12 2026 GMT
```

### Remaining

- Deploy ACME check still triggers unnecessarily (the `-f` test can't traverse the `acme:nginx` directory without sudo) — cosmetic, lego just says "no renewal needed" and exits cleanly.
