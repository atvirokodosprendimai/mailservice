# Gift Coupon Marketing Copy

Created: 2026-03-16
Coupon code: `OPENCLAWS` (3 months free via Polar discount)

---

## Reddit (r/selfhosted, r/LocalLLaMA, r/SideProject)

**Title:** Your AI agent needs an email address — here's one in 6 curl calls

**Body:**

Built an inbound email service for LLM agents. An Ed25519 key IS the mailbox identity — same key, same mailbox, forever. No passwords, no OAuth.

```bash
# 1. generate a key (or reuse one — same key = same mailbox)
ssh-keygen -t ed25519 -f identity -N "" -C "agent@context"
PUBKEY=$(cat identity.pub)

# 2. get a challenge (30s TTL)
CHALLENGE=$(curl -s -X POST https://truevipaccess.com/v1/auth/challenge \
  -d '{"public_key": "'"$PUBKEY"'"}' | jq -r .challenge)

# 3. sign it
SIGNATURE=$(echo -n "$CHALLENGE" | ssh-keygen -Y sign -f identity -n edproof \
  | sed '1d;$d' | tr -d '\n')

# 4. claim a mailbox (coupon = 3 months free)
curl -s -X POST https://truevipaccess.com/v1/mailboxes/claim \
  -d '{"billing_email":"YOUR_EMAIL","edproof":"'"$PUBKEY"'","challenge":"'"$CHALLENGE"'","signature":"'"$SIGNATURE"'","coupon_code":"OPENCLAWS"}'

# → returns mailbox address + payment_url (skip payment with coupon)

# 5. after activation, get a new challenge + sign it, then resolve access
CHALLENGE2=$(curl -s -X POST https://truevipaccess.com/v1/auth/challenge \
  -d '{"public_key": "'"$PUBKEY"'"}' | jq -r .challenge)
SIG2=$(echo -n "$CHALLENGE2" | ssh-keygen -Y sign -f identity -n edproof \
  | sed '1d;$d' | tr -d '\n')

curl -s -X POST https://truevipaccess.com/v1/access/resolve \
  -d '{"protocol":"imap","edproof":"'"$PUBKEY"'","challenge":"'"$CHALLENGE2"'","signature":"'"$SIG2"'"}'

# → returns IMAP host, port, username, password — or use the HTTP API:

# 6. read mail over HTTP
curl -s -X POST https://truevipaccess.com/v1/imap/messages \
  -d '{"access_token":"TOKEN_FROM_STEP_5","unread_only":true,"include_body":true}'
```

**Why?** Temp mail expires. Gmail API is overkill. Self-hosted Postfix is too much ops for "my agent needs to receive a confirmation email."

Normally 1 EUR/month. Giving away **3 months free** to early adopters — coupon `OPENCLAWS` is already in the script above, or enter it at checkout on [truevipaccess.com](https://truevipaccess.com).

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
