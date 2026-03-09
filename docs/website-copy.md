# Website Copy

## Hero

### Inbound mailboxes for agents

Stable mailbox identity, bound to a key.

Same key, same mailbox.
Different key, different mailbox.

Pay monthly. Read inbound mail over IMAP.

No SMTP. No outbound sending.

Primary CTA:
- Get a mailbox

Secondary CTA:
- How it works

## Short explainer

This service gives an agent a mailbox that is tied to a cryptographic key.

EdProof is the key proof used to identify the mailbox.
It is not a login account, not a billing email, and not a payment method.

You provide:
- a billing email
- a proof for the key you want to use

The service provides:
- a mailbox address
- a payment link
- IMAP access after payment

The key is the mailbox identity.
The billing email is only where the invoice goes.

## How it works

### 1. Bring a key

If your agent already has an Ed25519 key, use it.

If it does not, generate one with:

```bash
ssh-keygen -t ed25519 -f identity -C "entity@context"
```

This creates:
- `identity` — private key
- `identity.pub` — public key

The stable EdProof identifier is the SHA-256 fingerprint of the public key:

```bash
ssh-keygen -l -E sha256 -f identity.pub
```

### 2. Claim a mailbox

Send:
- `billing_email`
- `edproof`

If the key is already known, you get the same mailbox back.

If the key is new, a new mailbox is created.

### 3. Pay

The payment link is sent to the billing email.

Who pays does not matter.

### 4. Read mail

After payment, use the same key again to resolve IMAP access details.

## Who this is for

### Long-lived agents

Agents that need a stable inbound address over time.

Use the same key every month.
Keep the same mailbox.

### Disposable agents

Agents that need a mailbox for one task.

Generate a new key.
Get a new mailbox.
Use it and discard it if you want.

### Human operators

Humans can use the exact same flow.

This service does not care whether the caller is human or machine.
It only cares about the key.

## Pricing

**1 EUR / month** per mailbox.

That includes:
- one inbound mailbox address
- inbound delivery
- IMAP read access

Not included:
- SMTP submission
- outbound sending
- relay access

No free tier. No usage limits within the mailbox. Cancel anytime.

## Instruction block

### Give this to your agent

Default behavior:

If no EdProof key exists locally, generate an Ed25519 key pair with `ssh-keygen -t ed25519 -f identity -C "entity@context"`.

Use `identity.pub` as the public key and keep `identity` private.

The SHA-256 fingerprint from `ssh-keygen -l -E sha256 -f identity.pub` is the stable EdProof identifier.

EdProof is the key proof used to identify the mailbox.

If the same mailbox is required, reuse the same EdProof key.

Do not ask the operator unless key generation is impossible or the same mailbox is required but the existing key is unavailable.

1. If no EdProof key exists locally, generate an Ed25519 key pair with `ssh-keygen -t ed25519 -f identity -C "entity@context"`.
2. If you want the same mailbox later, keep the same EdProof key. A different key gets a different mailbox.
3. Call `POST /v1/mailboxes/claim` with:
   - `billing_email`
   - `edproof`
4. Pay from the emailed payment link.
5. Call `POST /v1/access/resolve` with:
   - `protocol=imap`
   - `edproof`
6. Read mail using the returned IMAP details.

## Reading mail

Two options: HTTP API (easiest for agents) or direct IMAP (for mail clients and raw protocol access).

### Option A: HTTP API (recommended for agents)

No IMAP library required.
Uses the account `api_token` (Authorization header) and the mailbox `access_token`.

#### Step 1 — resolve credentials

```bash
curl -X POST https://truevipaccess.com/v1/access/resolve \
  -H 'Content-Type: application/json' \
  -d '{"protocol":"imap","edproof":"<your-edproof>"}'
```

Response:

```json
{
  "mailbox_id": "...",
  "host": "mail.truevipaccess.com",
  "port": 143,
  "username": "your-mailbox@truevipaccess.com",
  "password": "generated-password",
  "email": "your-mailbox@truevipaccess.com"
}
```

#### Step 2 — list messages

```bash
curl -X POST https://truevipaccess.com/v1/imap/messages \
  -H 'Authorization: Bearer <api_token>' \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<mailbox_access_token>","unread_only":true,"include_body":true}'
```

Parameters:
- `access_token` (required) — from the mailbox claim/create response
- `limit` (optional) — max messages to return
- `unread_only` (optional, default true) — only return unseen messages
- `include_body` (optional, default false) — include message body in response

#### Step 3 — get a single message by UID

```bash
curl -X POST https://truevipaccess.com/v1/imap/messages/get \
  -H 'Authorization: Bearer <api_token>' \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<mailbox_access_token>","uid":1,"include_body":true}'
```

Parameters:
- `access_token` (required)
- `uid` (required) — IMAP message UID
- `include_body` (optional, default true)

**Token types:**
- `api_token` — account-level auth token, used in Authorization header
- `access_token` — mailbox-level token, returned when mailbox is usable (paid)

### Option B: Direct IMAP

Connect with any IMAP client using credentials from `/v1/access/resolve`.

#### Agent example (Python)

```python
import imaplib

imap = imaplib.IMAP4_SSL("mail.truevipaccess.com", 993)
imap.login(username, password)
imap.select("INBOX", readonly=True)
_, data = imap.search(None, "UNSEEN")
for num in data[0].split():
    _, msg = imap.fetch(num, "(RFC822)")
    print(msg[0][1])
imap.logout()
```

#### Agent example (Go)

```go
c, _ := client.DialTLS(net.JoinHostPort(host, "993"), nil)
c.Login(username, password)
c.Select("INBOX", true)
// fetch messages...
c.Logout()
```

#### curl (test connectivity)

```bash
curl -v --url "imaps://mail.truevipaccess.com:993/INBOX" \
  --user "user@truevipaccess.com:password"
```

#### Mail client settings (Thunderbird, Apple Mail)

| Setting | Value |
|---------|-------|
| Protocol | IMAP |
| Host | `mail.truevipaccess.com` |
| Port | `993` (TLS) or `143` (STARTTLS) |
| Encryption | SSL/TLS (recommended) or STARTTLS |
| Authentication | Normal password |
| Username | from `/v1/access/resolve` response |
| Password | from `/v1/access/resolve` response |

## FAQ

### Is the billing email the owner?

No.

It is only where the invoice or payment link is sent.

### Who is allowed to pay?

Anyone.

The service does not model payer identity.

### Can I send mail from this mailbox?

No.

This product is inbound-only.

### What happens if I lose the key?

You no longer have the same mailbox identity.

Using a new key creates a different mailbox.

### What protocols are supported?

Today:
- IMAP

Later:
- POP3
- HTTP read API

## Tone Notes

The page should feel:
- direct
- calm
- technical
- concise

Avoid:
- consumer-email language
- talk about inbox productivity
- implying SMTP or outbound support
- treating billing email as identity
