---
tldr: Full E2E production test of claim → pay → activate → resolve → read flow, all passing
---

# Solved — End-to-end production smoke test passes

## Date
2026-03-10

## What was tested

Full user journey on production (`truevipaccess.com` / `mail.truevipaccess.com`).

### Infrastructure

| Component | Status | Details |
|-----------|--------|---------|
| API `/healthz` | PASS | `{"status":"ok"}` |
| Landing page `/` | PASS | HTTP 200 |
| DNS MX | PASS | `10 mail.truevipaccess.com.` |
| DNS A | PASS | `46.62.133.191` |
| SMTP (port 25) | PASS | Postfix EHLO, 50MB limit |
| IMAPS (port 993) | PASS | Dovecot, Let's Encrypt cert valid until Jun 7 |

### Application flow

| Step | Status | Details |
|------|--------|---------|
| Generate Ed25519 key | PASS | Fingerprint derived |
| `POST /v1/mailboxes/claim` | PASS | 201, Polar checkout created |
| Payment email sent (Unsend) | PASS | Delivered via SES, DKIM signed for `truevipaccess.com` |
| Mail delivery (Postfix → Dovecot) | PASS | Payment email arrived in mailbox IMAP inbox |
| Polar checkout completed | PASS | Payment processed |
| Webhook → mailbox activation | PASS | Status → active, subscription set |
| `POST /v1/access/resolve` | PASS | 200, returns host/port/username/password/access_token |
| IMAP login (TLS, port 993) | PASS | Dovecot authenticates activated mailbox |
| `POST /v1/imap/messages` (HTTP API) | PASS | Returns messages, `{"status":"ok"}` |

### Self-referencing flow tested

Claimed a mailbox with `billing_email` set to an existing mailbox address (`mbx_180d1d9c0f4@truevipaccess.com`).
The payment link email was delivered to that same mailbox and readable via IMAP — proving the full circular flow works.

## Issues found and fixed

1. **Polar API token expired** — org-level OAT secret was missing from repo.
   Resolved by setting `POLAR_TOKEN` via `gh secret set` (interactive, not pasted in chat — Polar auto-revokes leaked tokens).

2. **Polar checkout API changed** — deprecated `POST /v1/checkouts/custom/` with `product_price_id`, now requires `POST /v1/checkouts/` with `products` array.
   Fixed in `task/polar-checkout-api-v2` branch: renamed `POLAR_PRICE_ID` → `POLAR_PRODUCT_ID`, updated gateway, config, deploy workflow, tests, and docs.

## Conclusion

All three access paths (IMAP direct, HTTP API, email notification) work end-to-end in production.
The service is fully operational.
