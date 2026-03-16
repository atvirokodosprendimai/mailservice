# Gift Coupon Marketing Copy

Created: 2026-03-16
Coupon code: `OPENCLAWS` (3 months free via Polar discount)

---

## Reddit (r/selfhosted, r/LocalLLaMA, r/SideProject)

**Title:** Your AI agent needs an email. Here's one in 4 lines.

**Body:**

```
curl -X POST https://truevipaccess.com/v1/auth/challenge \
  -d '{"public_key": "'$(cat agent.pub)'"}'

# sign the challenge, claim a mailbox, read mail over IMAP or HTTP.
# Ed25519 key = identity. No passwords. No OAuth.
```

Built an inbound email service for LLM agents. Each agent generates an Ed25519 key pair — the key IS the mailbox identity. Same key, same mailbox, forever.

**Why?** Temp mail expires. Gmail API is overkill. Self-hosted Postfix is too much ops for "my agent needs to receive a confirmation email."

Normally 1 EUR/month. Giving away **3 months free** to early adopters:

**`OPENCLAWS`** — enter at checkout on [truevipaccess.com](https://truevipaccess.com)

Open source (AGPL v3): [github.com/atvirokodosprendimai/mailservice](https://github.com/atvirokodosprendimai/mailservice)

---

## Moltbook

Your AI agent needs an email address. Not a temp one that dies in 10 minutes.

```
curl POST /v1/auth/challenge -d '{"public_key": "ed25519:..."}'
```

Ed25519 key = stable mailbox identity. No passwords, no OAuth, inbound only.

3 months free with code **OPENCLAWS** → truevipaccess.com

Open source: github.com/atvirokodosprendimai/mailservice
