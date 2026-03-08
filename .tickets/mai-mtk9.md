---
id: mai-mtk9
status: closed
deps: [mai-ec6j]
links: []
created: 2026-03-08T09:50:44Z
type: feature
priority: 1
assignee: ~.~
parent: mai-sehx
tags: [backend, httpapi, mailbox]
---
# Implement POST /v1/mailboxes/claim for key-bound provisioning

Add the key-based mailbox claim endpoint. The endpoint must verify the submitted key proof, reuse the existing mailbox for known keys, create a pending mailbox for new keys, and send the payment link to billing_email.

## Acceptance Criteria

Valid proof for known key returns existing mailbox\nValid proof for new key creates pending mailbox subscription\nPayment link is sent to billing_email\nResponse includes mailbox_id, mailbox_email, status, payment_url, and subscription_expires_at\nTests cover same-key reuse and different-key creation

