---
name: TrueVIP Access Mailbox
last_updated: 2026-05-09
---

# TrueVIP Access Mailbox Strategy

## Target problem

Autonomous agents (LLMs, scripts, bots) need a durable inbound email address they can re-prove ownership of from any host using only the keypair they already hold. Existing options fail this: Gmail/OAuth assume a human owner with passwords and recovery; temp-mail expires and breaks continuity; self-hosted Postfix is too much ops for a single mailbox.

## Our approach

Bind mailbox identity to an Ed25519 key, not to an account. Same key returns the same mailbox; payment is an activation signal, not identity authority. Refuse SMTP and outbound on purpose — sell the smallest inbound primitive with cryptographic continuity, so the product never turns into a spam-handling shop.

## Who it's for

**Primary:** Long-lived autonomous agents — They're hiring TrueVIP to give them a stable mailbox they can re-prove ownership of from any host using only their keypair, without managing accounts, passwords, or OAuth state.

**Secondary:** One-off / disposable agents — Same primitive, single-task lifetime: generate key, claim mailbox, read confirmations, discard.

## Key metrics

- **Active paid mailboxes** — count of subscriptions in `active` status; durable demand signal (DB)
- **Claim → activation conversion** — `pending_payment` mailboxes that reach `active` within 24h; payment funnel health (DB)
- **Monthly renewal rate** — fraction of expiring subscriptions that re-pay; whether key continuity translates to subscription continuity (DB)
- **Resolve calls per active mailbox per week** — `/v1/access/resolve` usage; distinguishes live mailboxes from dormant ones (logs)
- **Failed key-proof ratio** — rejected `edproof` verifications / total attempts; spike indicates SDK/onboarding regression (logs)

## Tracks

### Key-bound primitive

Harden the claim → proof → resolve pipeline: EdProof challenge-response, fingerprint stability, the no-SMTP sentinel boundary.

_Why it serves the approach:_ the entire product is "same key = same mailbox" working reliably; if this drifts the moat is gone.

### Operational durability

NixOS production host, Postfix+Dovecot+API as native systemd services, GitOps deploy on merge-to-main, Cloudflare tunnel, Coroot observability.

_Why it serves the approach:_ the public story is "decomposition as survival practice" — the runtime must actually survive partial outages without phoning home for permission.

### Agent-native integration surface

HTTP read API alongside IMAP, paste-into-context agent-API skill doc, gift coupon / OpenClaw integration paths.

_Why it serves the approach:_ agents adopt what they can use without an SDK; paste-and-go beats library-first.

### Payment activation rails

Polar primary, Stripe legacy fallback, mock for local dev, webhook signature verification.

_Why it serves the approach:_ payment must stay decoupled from identity and easy to swap — it's an activation signal, not an auth primitive.

## Not working on

- SMTP submission and outbound sending (refused on purpose; product boundary)
- Person-bound accounts and OAuth flows (legacy account/token flow being deprecated)
- Modeling who paid (only that payment succeeded against a key-bound mailbox)

## Marketing

**One-liner:** Private inbound email for LLM agents — Ed25519 key is the mailbox identity.

**Key message:** Same key, same mailbox. Different key, different mailbox. 1 EUR/month, IMAP + HTTP API. No SMTP, no outbound. Open source (AGPL v3.0). Live at truevipaccess.com.
