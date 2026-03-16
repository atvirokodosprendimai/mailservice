# Show HN: I built a mailbox service for LLM agents

**Title:** Show HN: Private email for LLM agents — Ed25519 key = stable mailbox identity

**URL:** https://truevipaccess.com

**Text:**

I built an inbound email service designed for AI agents.

**The problem:** Agents need email addresses to sign up for services, receive confirmations, and get notifications. Temp mail is too ephemeral — addresses expire and you lose continuity. Gmail API is overkill and not agent-native. Self-hosted Postfix is too much ops for a simple mailbox.

**The solution:** Each agent generates an Ed25519 key pair. The public key becomes the mailbox identity — same key always maps to the same mailbox. Agents prove key ownership via a challenge-response flow, then read mail over IMAP or a simple HTTP API.

**How it works:**
1. Agent calls `POST /v1/auth/challenge` with its public key
2. Signs the challenge with its private key
3. Claims a mailbox via `POST /v1/mailboxes/claim`
4. After payment (1 EUR/month), reads mail via `POST /v1/imap/messages`

**Why Ed25519 keys?** No passwords to manage. No OAuth dance. The key IS the identity — deterministic, portable, and the agent already knows how to use it. If the key is lost, the mailbox is inaccessible (by design — this is a feature, not a bug).

**Anti-abuse by design:** Inbound only (no SMTP), payment required, key proof required. There's no free tier to abuse.

**Tech:** Go, hexagonal architecture, SQLite, Polar for billing. The full API skill document is at `GET /docs/agent-api-skill.md` — you can literally paste it into an agent's context and it knows how to use the service.

Open source: https://github.com/atvirokodosprendimai/mailservice (AGPL v3.0)

---

## Reddit variant (r/selfhosted, r/LocalLLaMA)

**Title:** I built a mailbox service for LLM agents — Ed25519 key-bound identity, 1 EUR/month, open source

**Text:** Same as above, add: "Self-hostable if you want to run your own instance. AGPL v3.0."
