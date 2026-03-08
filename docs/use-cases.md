# Use Cases

This service sells inbound mailboxes bound to cryptographic keys.

The rule is simple:

- same key, same mailbox
- different key, different mailbox
- billing email is where the payment link goes
- who actually pays does not matter
- only inbound read access is sold

## 1. Long-Lived Agent

An agent needs a stable mailbox identity over time.

Flow:
1. Generate or reuse a key.
2. Claim mailbox with `billing_email` and key proof.
3. Pay the monthly invoice.
4. Use the same key to resolve IMAP access details.

Result:
- the mailbox stays stable as long as the same key is used
- the billing email can stay the same or be updated in future claim/renew flows

This is the primary use case.

## 2. One-Off Agent

An agent needs a mailbox now and does not care about keeping it later.

Flow:
1. Generate a new key.
2. Claim mailbox with that key.
3. Pay.
4. Read inbound mail.

Result:
- the mailbox is tied to that temporary key
- if the key is discarded, access is effectively gone
- generating a new key later creates a different mailbox

## 3. Renewal With The Same Key

An agent already has a mailbox and wants to keep it.

Flow:
1. Use the same key again.
2. Receive renewal/payment request.
3. Pay for another month.
4. Continue resolving access with the same key.

Result:
- same key returns the same mailbox
- subscription is extended
- mailbox identity does not change

## 4. Billing Email Is Not Identity

The billing address is only where the invoice or payment link is sent.

Examples:
- owner pays
- teammate pays
- accounting mailbox pays
- a family member pays

The service does not model who paid.

It only cares that:
- a payment link was delivered
- payment succeeded
- the mailbox bound to the key is active

## 5. Inbound-Only Mailbox

This service is for receiving and reading mail.

Included:
- inbound delivery to the mailbox address
- IMAP access today
- POP3 later if added
- HTTP read API later if added

Not included:
- SMTP submission
- outbound sending
- relay access

## 6. Human Or Non-Human Agent

The service does not distinguish between a human-operated client and an autonomous agent.

The workflow is the same:
1. have a key or generate one
2. prove control of that key
3. claim mailbox
4. pay
5. use the same key for access

This is intentional. The product is key-bound, not person-bound.

## 7. What To Tell Integrators

If you integrate with this service, the practical instructions are:

1. Generate a key if you do not already have one.
2. Keep that key if you want the same mailbox in the future.
3. Call `POST /v1/mailboxes/claim` with:
   - `billing_email`
   - `edproof`
4. Complete payment from the emailed link.
5. Call `POST /v1/access/resolve` with:
   - `protocol`
   - `edproof`
6. Use the returned IMAP credentials to read mail.

If you use a new key, you get a new mailbox.
