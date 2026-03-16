# OpenClaw Integration Demo

End-to-end flow showing an OpenClaw agent claiming and using a mailbox.

## Prerequisites

- OpenClaw agent with shell access
- `ssh-keygen`, `curl`, `jq` available
- OPENCLAWS coupon code (23 free slots available)

## Step 1: Generate a key

```bash
ssh-keygen -t ed25519 -f identity -N "" -C "openclaw@agent"
```

The agent now has `identity` (private) and `identity.pub` (public).

## Step 2: Get a challenge and sign it

```bash
PUBKEY=$(cat identity.pub)
BASE="https://truevipaccess.com"

# Get challenge
CHALLENGE=$(curl -sf -X POST "$BASE/v1/auth/challenge" \
  -H 'Content-Type: application/json' \
  -d "{\"public_key\": \"$PUBKEY\"}" | jq -r '.challenge')

# Sign it (strip SSHSIG armor, join lines)
SIGNATURE=$(echo -n "$CHALLENGE" \
  | ssh-keygen -Y sign -f identity -n edproof 2>/dev/null \
  | sed '1d;$d' | tr -d '\n')
```

## Step 3: Claim a mailbox with OPENCLAWS coupon

```bash
CLAIM=$(curl -sf -X POST "$BASE/v1/mailboxes/claim" \
  -H 'Content-Type: application/json' \
  -d "{
    \"billing_email\": \"agent-operator@example.com\",
    \"edproof\": \"$PUBKEY\",
    \"challenge\": \"$CHALLENGE\",
    \"signature\": \"$SIGNATURE\",
    \"coupon_code\": \"OPENCLAWS\"
  }")

echo "$CLAIM" | jq .
```

Expected response — mailbox is immediately active (no payment needed):

```json
{
  "id": "mbx_abc123",
  "status": "active",
  "usable": true,
  "payment_url": "",
  "expires_at": "2026-06-16T00:00:00Z",
  "access_token": "tok_..."
}
```

## Step 4: Sign up for a service using the mailbox email

The agent's email address is `mbx_abc123@truevipaccess.com` (from the claim response or resolve).

```bash
# Example: sign up for a service
curl -X POST "https://some-service.com/signup" \
  -d "email=mbx_abc123@truevipaccess.com&name=OpenClaw+Agent"
```

## Step 5: Read the confirmation email

```bash
ACCESS_TOKEN=$(echo "$CLAIM" | jq -r '.access_token')

# Wait a moment for delivery, then check
MESSAGES=$(curl -sf -X POST "$BASE/v1/imap/messages" \
  -H 'Content-Type: application/json' \
  -d "{\"access_token\": \"$ACCESS_TOKEN\", \"unread_only\": true, \"include_body\": true}")

echo "$MESSAGES" | jq '.messages[] | {from, subject, body}'
```

## What the agent sees

```json
{
  "from": "noreply@some-service.com",
  "subject": "Confirm your email",
  "body": "Click here to confirm: https://some-service.com/confirm?token=abc123"
}
```

The agent extracts the confirmation link and follows it. Done — autonomous email-based signup.

## Key points for the demo

- **No human intervention** from key generation through email reading
- **Coupon code** makes it free for OpenClaw agents (3 months)
- **Same key = same mailbox** across agent restarts
- **HTTP API** means no IMAP library needed — just curl/requests
