# TrueVIP Access Mailbox — Agent API Skill

> **Audience:** AI agents that need a stable inbound email address.
> **Base URL:** Replace `$BASE` with the service URL (e.g. `https://truevipaccess.com`).
> **Product:** Inbound-only mailboxes bound to Ed25519 keys. Same key = same mailbox. No SMTP. No outbound.
> **Pricing:** 1 EUR/month per mailbox, 100 MB storage.

---

## Quick Reference

| Endpoint | Method | Auth | Purpose |
|---|---|---|---|
| `/v1/auth/challenge` | POST | none | Get a challenge string for signing |
| `/v1/mailboxes/claim` | POST | challenge-response | Claim or retrieve a mailbox |
| `/v1/access/resolve` | POST | challenge-response | Get IMAP credentials |
| `/v1/imap/messages` | POST | access_token | List messages |
| `/v1/imap/messages/get` | POST | access_token | Get a single message by UID |
| `/v1/support/messages` | POST | challenge-response | Send a message to support |

---

## Prerequisites

### Do I already have an Ed25519 key?

- **Yes** → skip to [Step 1: Get a Challenge](#step-1-get-a-challenge).
  Use the existing `identity` (private) and `identity.pub` (public) files.
- **No** → generate one:

```bash
ssh-keygen -t ed25519 -f identity -N "" -C "agent@context"
```

This creates:
- `identity` — private key (never share)
- `identity.pub` — public key (send to the API)

Read the public key:

```bash
cat identity.pub
# Output: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... agent@context
```

**Rule:** Same key = same mailbox. If you need the same mailbox across sessions, persist and reuse the key pair. A different key creates a different mailbox.

---

## Full Lifecycle

### Step 1: Get a Challenge

```bash
curl -s -X POST "$BASE/v1/auth/challenge" \
  -H 'Content-Type: application/json' \
  -d "{\"public_key\": \"$(cat identity.pub)\"}"
```

Response:

```json
{
  "challenge": "v1.1741689600.a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4.hmac_hex",
  "expires_in": 30
}
```

**The challenge expires in 30 seconds.** You must sign and submit it before it expires. If it expires, request a new one.

Save the challenge string:

```bash
CHALLENGE="<challenge string from response>"
```

### Step 2: Sign the Challenge

**Option A: ssh-keygen (recommended)**

```bash
echo -n "$CHALLENGE" | ssh-keygen -Y sign -f identity -n edproof
```

This outputs an armored SSHSIG block:

```
-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQ...
...multiple lines of base64...
-----END SSH SIGNATURE-----
```

**Extract the base64 payload — strip the armor and join into a single line:**

```bash
SIGNATURE=$(echo -n "$CHALLENGE" | ssh-keygen -Y sign -f identity -n edproof 2>/dev/null \
  | sed '1d;$d' | tr -d '\n')
```

**Critical:** The API expects the raw base64 content without `-----BEGIN SSH SIGNATURE-----` / `-----END SSH SIGNATURE-----` headers and without newlines. Multi-line base64 will be rejected.

**Critical:** The namespace must be `edproof` (the `-n edproof` flag). Any other namespace will be rejected.

**Option B: Raw Ed25519 signature (programmatic)**

If you have the raw Ed25519 private key bytes, sign the challenge bytes directly and base64-encode the 64-byte signature:

```python
import base64
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

# Sign the raw challenge bytes
signature = private_key.sign(challenge.encode())
sig_b64 = base64.b64encode(signature).decode()
```

### Step 3: Claim a Mailbox

```bash
curl -s -X POST "$BASE/v1/mailboxes/claim" \
  -H 'Content-Type: application/json' \
  -d "{
    \"billing_email\": \"you@yourdomain.com\",
    \"edproof\": \"$(cat identity.pub)\",
    \"challenge\": \"$CHALLENGE\",
    \"signature\": \"$SIGNATURE\"
  }"
```

Response (new mailbox):

```json
{
  "id": "mbx_abc123",
  "status": "pending_payment",
  "usable": false,
  "payment_url": "https://checkout.stripe.com/...",
  "access_token": ""
}
```

Response (existing paid mailbox):

```json
{
  "id": "mbx_abc123",
  "status": "active",
  "usable": true,
  "payment_url": "",
  "expires_at": "2026-04-11T00:00:00Z",
  "access_token": "tok_..."
}
```

**Decision tree after claim:**

- `"usable": true` → skip to [Step 5: Read Messages](#step-5-read-messages). The `access_token` is in the response.
- `"usable": false` and `"status": "pending_payment"` → proceed to [Step 4: Pay](#step-4-pay).
- `"status": "expired"` → the `payment_url` is a renewal link. Pay to reactivate.

### Step 4: Pay

The `payment_url` from the claim response is a Stripe/Polar checkout link. Payment cannot be completed programmatically by the agent — it requires a human or a browser session.

**What to do:**

1. Present the `payment_url` to the operator/user for payment.
2. Wait for payment to complete. The mailbox status changes to `active` after payment.
3. After payment, proceed to Step 5.

**How to check if payment completed:**

Request a new challenge (Step 1), sign it (Step 2), then call `/v1/access/resolve` (Step 5). If the mailbox is active, you'll get IMAP credentials. If still pending, you'll get a `409 Conflict` with `{"status": "waiting_payment"}`.

### Step 5: Resolve Access (Get IMAP Credentials)

**You need a fresh challenge for this step** — challenges are single-use in practice (they expire in 30 seconds).

```bash
# Get a new challenge
CHALLENGE=$(curl -s -X POST "$BASE/v1/auth/challenge" \
  -H 'Content-Type: application/json' \
  -d "{\"public_key\": \"$(cat identity.pub)\"}" | jq -r '.challenge')

# Sign it
SIGNATURE=$(echo -n "$CHALLENGE" | ssh-keygen -Y sign -f identity -n edproof 2>/dev/null \
  | sed '1d;$d' | tr -d '\n')

# Resolve access
curl -s -X POST "$BASE/v1/access/resolve" \
  -H 'Content-Type: application/json' \
  -d "{
    \"protocol\": \"imap\",
    \"edproof\": \"$(cat identity.pub)\",
    \"challenge\": \"$CHALLENGE\",
    \"signature\": \"$SIGNATURE\"
  }"
```

Response:

```json
{
  "mailbox_id": "mbx_abc123",
  "host": "mail.truevipaccess.com",
  "port": 143,
  "username": "mbx_abc123@truevipaccess.com",
  "password": "generated_password",
  "email": "mbx_abc123@truevipaccess.com",
  "access_token": "tok_..."
}
```

Save `access_token` — you'll use it for reading messages without needing to redo the challenge flow.

### Step 6: Read Messages

Once you have an `access_token`, use the HTTP API to read messages. No IMAP library needed.

**List messages:**

```bash
curl -s -X POST "$BASE/v1/imap/messages" \
  -H 'Content-Type: application/json' \
  -d "{
    \"access_token\": \"$ACCESS_TOKEN\",
    \"unread_only\": true,
    \"include_body\": true
  }"
```

Response:

```json
{
  "status": "ok",
  "provider": "imap",
  "messages": [
    {
      "uid": 1,
      "from": "sender@example.com",
      "to": "mbx_abc123@truevipaccess.com",
      "subject": "Hello",
      "date": "2026-03-11T10:00:00Z",
      "body": "Message body text..."
    }
  ]
}
```

Optional parameters:
- `"unread_only"`: boolean (default `true`) — only return unseen messages
- `"include_body"`: boolean (default `false`) — include message body text
- `"limit"`: integer — max messages to return

**Get a single message by UID:**

```bash
curl -s -X POST "$BASE/v1/imap/messages/get" \
  -H 'Content-Type: application/json' \
  -d "{
    \"access_token\": \"$ACCESS_TOKEN\",
    \"uid\": 1,
    \"include_body\": true
  }"
```

Response:

```json
{
  "status": "ok",
  "provider": "imap",
  "message": {
    "uid": 1,
    "from": "sender@example.com",
    "to": "mbx_abc123@truevipaccess.com",
    "subject": "Hello",
    "date": "2026-03-11T10:00:00Z",
    "body": "Full message body..."
  }
}
```

---

## Contact Support

If you need help, you can send a message to the support team via HTTP. No email sending required — authenticate with EdProof and POST.

```bash
# Get a fresh challenge
CHALLENGE=$(curl -s -X POST "$BASE/v1/auth/challenge" \
  -H 'Content-Type: application/json' \
  -d "{\"public_key\": \"$(cat identity.pub)\"}" | jq -r '.challenge')

# Sign it
SIGNATURE=$(echo -n "$CHALLENGE" | ssh-keygen -Y sign -f identity -n edproof 2>/dev/null \
  | sed '1d;$d' | tr -d '\n')

# Send support message
curl -s -X POST "$BASE/v1/support/messages" \
  -H 'Content-Type: application/json' \
  -d "{
    \"edproof\": \"$(cat identity.pub)\",
    \"challenge\": \"$CHALLENGE\",
    \"signature\": \"$SIGNATURE\",
    \"subject\": \"Brief description of the issue\",
    \"body\": \"Detailed explanation of what happened and what you expected.\"
  }"
```

Response:

```json
{"status": "sent"}
```

**Constraints:**
- `subject`: required, max 200 characters
- `body`: required, max 10,000 characters
- Rate limit: 3 messages per hour per key
- Any mailbox status (active, pending, expired) can contact support
- The support team can reply directly to your mailbox email address

---

## Error Reference

| HTTP Status | Error Message | Cause | Action |
|---|---|---|---|
| 400 | `invalid public key format` | Public key is not in `ssh-ed25519 AAAA...` format | Check that you're sending the full public key line from `identity.pub` |
| 400 | `unsupported key type "..."` | Key is not Ed25519 | Generate an Ed25519 key: `ssh-keygen -t ed25519 -f identity` |
| 400 | `edproof now requires challenge-response — call POST /v1/auth/challenge first` | Missing `challenge` or `signature` field | Get a challenge first, sign it, include both fields |
| 400 | `unsupported protocol` | Protocol is not `imap` | Use `"protocol": "imap"` |
| 500 | `create payment link: polar api...` | Billing email domain doesn't accept email (e.g. `example.com`) | Use a real email address with a valid, deliverable domain |
| 400 | `uid must be > 0` | UID is 0 or missing | Provide a valid message UID from the list response |
| 401 | `challenge expired` | More than 30 seconds passed since challenge was issued | Request a new challenge and sign it again |
| 401 | `challenge tampered or invalid` | Challenge string was modified, or public key doesn't match | Use the exact challenge string from the response; use the same public key for challenge and claim/resolve |
| 401 | `challenge timestamp is in the future` | Server clock skew | Retry immediately — this is rare |
| 401 | `signature verification failed` | Signature doesn't match the challenge+key | Check: (1) signed the exact challenge string, (2) used `-n edproof` namespace, (3) stripped armor headers and newlines from SSHSIG output |
| 401 | `invalid key proof` | Public key verification failed | Ensure the public key is valid Ed25519 in SSH format |
| 404 | `mailbox not found` | No mailbox exists for this key | Claim a mailbox first with `/v1/mailboxes/claim` |
| 409 | `{"status": "waiting_payment"}` | Mailbox exists but payment is pending | Complete payment via the `payment_url`, then retry |
| 429 | `support message rate limit reached` | Sent 3+ support messages in the last hour | Wait and try again later |

---

## Common Mistakes

1. **Multi-line signature.** `ssh-keygen -Y sign` outputs base64 with newlines and armor headers. You must strip `-----BEGIN SSH SIGNATURE-----`, `-----END SSH SIGNATURE-----`, and join all base64 lines into one string with no whitespace.

2. **Wrong namespace.** The `-n edproof` flag is mandatory when using `ssh-keygen -Y sign`. Any other namespace (or omitting it) will be rejected.

3. **Reusing an expired challenge.** Challenges expire in 30 seconds. Always get a fresh challenge immediately before signing.

4. **Using a different key for challenge vs claim/resolve.** The challenge is HMAC-bound to the public key. You must use the same key for requesting the challenge and for the subsequent claim or resolve call.

5. **Trying to resolve access before paying.** `/v1/access/resolve` returns `409 Conflict` with `{"status": "waiting_payment"}` until the mailbox is paid.

6. **Sending the public key without the key type prefix.** The API expects the full SSH public key line: `ssh-ed25519 AAAA... comment`. Not just the base64 blob.

7. **Using `example.com` as billing email.** The payment provider validates that the email domain actually accepts mail. Use a real email address.

---

## End-to-End Example (Bash)

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE="https://truevipaccess.com"
KEY_FILE="identity"

# 1. Generate key if needed
if [ ! -f "$KEY_FILE" ]; then
  ssh-keygen -t ed25519 -f "$KEY_FILE" -N "" -C "agent@$(hostname)"
fi

PUBKEY=$(cat "${KEY_FILE}.pub")

# 2. Get challenge
CHALLENGE=$(curl -sf -X POST "$BASE/v1/auth/challenge" \
  -H 'Content-Type: application/json' \
  -d "{\"public_key\": \"$PUBKEY\"}" | jq -r '.challenge')

# 3. Sign challenge (strip SSHSIG armor, join lines)
SIGNATURE=$(echo -n "$CHALLENGE" \
  | ssh-keygen -Y sign -f "$KEY_FILE" -n edproof 2>/dev/null \
  | sed '1d;$d' | tr -d '\n')

# 4. Claim mailbox
CLAIM=$(curl -sf -X POST "$BASE/v1/mailboxes/claim" \
  -H 'Content-Type: application/json' \
  -d "{
    \"billing_email\": \"you@yourdomain.com\",
    \"edproof\": \"$PUBKEY\",
    \"challenge\": \"$CHALLENGE\",
    \"signature\": \"$SIGNATURE\"
  }")

echo "Claim response: $CLAIM"

USABLE=$(echo "$CLAIM" | jq -r '.usable')
if [ "$USABLE" = "true" ]; then
  ACCESS_TOKEN=$(echo "$CLAIM" | jq -r '.access_token')
  echo "Mailbox is active. Access token: $ACCESS_TOKEN"
else
  PAYMENT_URL=$(echo "$CLAIM" | jq -r '.payment_url')
  echo "Payment required: $PAYMENT_URL"
  echo "Complete payment, then run the resolve step."
  exit 0
fi

# 5. Read messages
MESSAGES=$(curl -sf -X POST "$BASE/v1/imap/messages" \
  -H 'Content-Type: application/json' \
  -d "{\"access_token\": \"$ACCESS_TOKEN\", \"unread_only\": true, \"include_body\": true}")

echo "Messages: $MESSAGES"
```

---

## Programmatic Example (Python)

```python
import subprocess, json, requests, base64, tempfile, os

BASE = "https://truevipaccess.com"
KEY_FILE = "identity"

# 1. Generate key if needed
if not os.path.exists(KEY_FILE):
    subprocess.run([
        "ssh-keygen", "-t", "ed25519", "-f", KEY_FILE, "-N", "", "-C", "agent"
    ], check=True)

with open(f"{KEY_FILE}.pub") as f:
    pubkey = f.read().strip()

# 2. Get challenge
resp = requests.post(f"{BASE}/v1/auth/challenge",
    json={"public_key": pubkey})
challenge = resp.json()["challenge"]

# 3. Sign challenge
with tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False) as tf:
    tf.write(challenge)
    tf_path = tf.name

result = subprocess.run(
    ["ssh-keygen", "-Y", "sign", "-f", KEY_FILE, "-n", "edproof"],
    stdin=open(tf_path, 'rb'),
    capture_output=True)
os.unlink(tf_path)

# Strip armor headers and join lines
lines = result.stdout.decode().strip().split('\n')
signature = ''.join(lines[1:-1])  # Remove first and last lines (armor)

# 4. Claim mailbox
resp = requests.post(f"{BASE}/v1/mailboxes/claim", json={
    "billing_email": "you@yourdomain.com",
    "edproof": pubkey,
    "challenge": challenge,
    "signature": signature,
})
claim = resp.json()

if claim.get("usable"):
    access_token = claim["access_token"]
    # 5. Read messages
    resp = requests.post(f"{BASE}/v1/imap/messages", json={
        "access_token": access_token,
        "unread_only": True,
        "include_body": True,
    })
    print(resp.json())
else:
    print(f"Payment required: {claim.get('payment_url')}")
```
