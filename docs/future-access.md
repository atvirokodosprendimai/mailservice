# Future Access Design

## Goal

Extend the current key-bound inbound mailbox model beyond IMAP without changing
the core authorization rule:

- valid proof of the bound key
- active mailbox subscription
- inbound-only access

## POP3

### Scope

Add POP3 as another inbound read protocol.

### Requirements

- caller presents key proof
- service resolves mailbox by key fingerprint
- service verifies mailbox is active
- response returns POP3 host, port, username, password, and mailbox email
- no SMTP submission details are returned

### Suggested API

- `POST /v1/access/resolve`
  with `protocol = "pop3"`

### Core behavior

The core service should keep protocol-neutral access resolution and let the
adapter map protocol to concrete host/port settings.

## HTTP Read API

### Scope

Expose read-only mailbox access over HTTP.

### Suggested endpoints

- `GET /v1/messages`
- `GET /v1/messages/{id}`
- `GET /v1/messages/{id}/raw`

### Requirements

- authenticate with the same key-bound proof model
- read only messages for the mailbox bound to that key
- return inbound mailbox content only
- no outbound send operations

### Message behavior

Likely operations:
- list recent messages
- fetch one message
- fetch raw RFC822 source
- optional search/filter later

## Shared Policy

All future access methods must preserve:
- same key => same mailbox
- different key => different mailbox
- no billing-email-based access
- no SMTP/outbound support

## Implementation Direction

Keep protocol-specific behavior in adapters:
- IMAP adapter today
- POP3 adapter later
- HTTP message adapter later

Keep authorization and entitlement in the core service:
- resolve mailbox by key
- verify mailbox status
- return protocol-neutral access result
