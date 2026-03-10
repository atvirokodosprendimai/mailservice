---
tldr: Automated smoke test every 5 min covering claim → pay → activate → resolve → read, using Polar sandbox
category: ops
---

# Todo: Periodic smoke test using Polar sandbox for full claim-to-read cycle

Create a smoke test that runs every 5 minutes and validates the full user journey end-to-end.

## What to test

1. `GET /healthz` — API is up
2. `POST /v1/mailboxes/claim` — claim succeeds, returns checkout URL
3. Payment via Polar **sandbox** (`sandbox-api.polar.sh`) with Stripe test card `4242 4242 4242 4242`
4. `POST /v1/access/resolve` — mailbox activates, returns IMAP credentials
5. IMAP login (port 993, TLS) — Dovecot authenticates
6. `POST /v1/imap/messages` — HTTP API returns messages

## Polar sandbox setup needed

- Create a sandbox org + product at [sandbox.polar.sh](https://sandbox.polar.sh/start)
- Generate a sandbox OAT (org access token) — production tokens don't work in sandbox
- The app needs a way to use sandbox for test claims (env var toggle or separate test endpoint)
- Sandbox subscriptions auto-cancel after 90 days

## Design considerations

- Should run as a GitHub Actions scheduled workflow (cron) or external monitor
- Use a **dedicated test key** so smoke test mailboxes don't pollute real data
- Alert on failure (GitHub Actions notification, or webhook to monitoring)
- Keep test mailboxes from accumulating — reuse same key (claim is idempotent for existing keys)
- Consider whether the smoke test hits production (with sandbox Polar) or a staging environment

## Reference

- Existing smoke test: `ops/smoke-test-mailbox.sh` (manual, requires human payment)
- Polar sandbox docs: https://polar.sh/docs/integrate/sandbox
- Stripe test cards: https://docs.stripe.com/testing#cards
