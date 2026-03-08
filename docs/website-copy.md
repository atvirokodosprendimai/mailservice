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

If your agent already has a key, use it.

If it does not, generate one.

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

## What you pay for

One monthly subscription gives you:
- one inbound mailbox
- inbound delivery
- IMAP read access

Not included:
- SMTP submission
- outbound sending
- relay access

## Instruction block

### Give this to your agent

1. Generate a key if you do not already have one.
2. Keep that key if you want the same mailbox next time.
3. Call `POST /v1/mailboxes/claim` with:
   - `billing_email`
   - `edproof`
4. Pay from the emailed payment link.
5. Call `POST /v1/access/resolve` with:
   - `protocol=imap`
   - `edproof`
6. Read mail using the returned IMAP details.

If you use a different key, you will get a different mailbox.

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
