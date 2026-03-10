---
tldr: Automated smoke test every 5 min covering claim → pay → activate → resolve → read, using Polar sandbox
category: ops
---

# Todo: Periodic smoke test using Polar sandbox for full claim-to-read cycle

Goal: every 5 minutes, verify the full flow works from intent-to-pay through getting the mailbox — ensuring no reason exists for a refund.

## What to test

1. `GET /healthz` — API is up
2. `POST /v1/mailboxes/claim` — claim succeeds, returns checkout URL
3. **Payment completes** — checkout confirmed, webhook fires, mailbox activates
4. `POST /v1/access/resolve` — returns IMAP credentials
5. IMAP login (port 993, TLS) — Dovecot authenticates
6. `POST /v1/imap/messages` — HTTP API returns messages

## Architecture constraint

The webhook handler (`handlePolarWebhook`) verifies the webhook signature AND calls `GetPaymentSession` on the Polar API to confirm the checkout actually succeeded.
This means the app must talk to the same Polar environment that created the checkout.
A sandbox checkout verified against production Polar will fail.

## Options

### A — Separate test deployment (recommended)

Deploy a second instance of the app configured with Polar sandbox:
- `POLAR_TOKEN` = sandbox OAT
- `POLAR_SERVER_URL` = `https://sandbox-api.polar.sh`
- `POLAR_PRODUCT_ID` = sandbox product ID
- `POLAR_WEBHOOK_SECRET` = sandbox webhook secret
- Same Postfix/Dovecot/IMAP (shared, or separate test domain)

Smoke test flow:
1. Claim against test instance → creates sandbox checkout
2. Use Polar sandbox API: `GET /v1/checkouts/{id}` → get `client_secret`
3. `POST /v1/checkouts/client/{client_secret}/confirm` → payment completes (sandbox, test card)
4. Polar sandbox sends webhook → test instance activates mailbox
5. Resolve → IMAP → messages

Pro: tests the real flow including Polar's webhook delivery.
Con: needs a second deployment.

### B — Webhook simulation with a smoke-test-only bypass

Add a `SMOKE_WEBHOOK_SECRET` env var.
The smoke test:
1. Claims → gets checkout ID
2. Sends a signed webhook to `/v1/webhooks/polar` with the checkout ID
3. App verifies signature, but instead of calling `GetPaymentSession`, checks a smoke-test flag

Pro: no second deployment.
Con: doesn't test the real Polar payment flow — skips the most critical part.

### C — Production app with dual Polar config

Add `POLAR_SANDBOX_*` config alongside production Polar.
Smoke test claims go through a `/v1/mailboxes/claim?sandbox=1` flag that routes to sandbox Polar.

Pro: single deployment, tests real Polar sandbox flow.
Con: adds complexity to production code, leaks test concerns into production.

## Current state

- `POLAR_SANDBOX_TOKEN` secret is set in GitHub
- Sandbox product needs to be created
- `ops/smoke-test-periodic.sh` exists but only tests the post-payment flow (reuses already-paid key)

## Reference

- Webhook handler: `internal/adapters/httpapi/handler.go:954`
- Webhook verification: `internal/adapters/httpapi/polar_webhook.go`
- Existing smoke test: `ops/smoke-test-periodic.sh`
- Polar sandbox docs: https://polar.sh/docs/integrate/sandbox
- Stripe test cards: https://docs.stripe.com/testing#cards
