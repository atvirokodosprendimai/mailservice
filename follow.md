# OpenClaw flow

Preferred key-bound flow:

1. Claim mailbox with billing email and key proof:

```bash
curl -X POST http://localhost:8080/v1/mailboxes/claim \
  -H 'Content-Type: application/json' \
  -d '{"billing_email":"billing@example.com","edproof":"<proof>"}'
```

- Same key returns the same mailbox.
- Different key returns a different mailbox.
- Service sends payment link to `billing_email`.

2. Complete payment and then resolve IMAP details with the same key:

If using Polar checkout, confirm the redirected checkout before resolving access:

```bash
curl "http://localhost:8080/v1/payments/polar/success?checkout_id=<polar-checkout-id>"

# production should use signed POST /v1/webhooks/polar with POLAR_WEBHOOK_SECRET
```

```bash
curl -X POST http://localhost:8080/v1/access/resolve \
  -H 'Content-Type: application/json' \
  -d '{"protocol":"imap","edproof":"<proof>"}'
```

- If not paid yet, API returns `409` with `{ "status": "waiting_payment" }`.
- On success the response includes `host`, `port`, `username`, `password`, `email`, and `access_token`.

3. Use the returned `access_token` to read mail via the HTTP API (no separate account token required):

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<access-token>","unread_only":true,"limit":20,"include_body":false}'
```

```bash
curl -X POST http://localhost:8080/v1/imap/messages/get \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<access-token>","uid":1,"include_body":true}'
```

Alternatively, connect to IMAP directly on port 143 using `username` and `password` from the resolve response.

Legacy account/token flow remains available during migration:

1. Register account once and get machine credentials:

```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Response includes:
- `api_token`
- `refresh_token`

2. Use `api_token` for regular bot API calls.

3. When token expires/rotates, refresh autonomously (no owner inbox needed):

```bash
curl -X POST http://localhost:8080/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh-token>"}'
```

Store returned new `api_token` and new `refresh_token`.

4. List existing mailboxes:

```bash
curl http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

5. Create mailbox:

```bash
curl -X POST http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

- If there is no pending mailbox, service creates one, sends payment link to owner email, and returns `201`.
- If a pending mailbox already exists, service returns it with `200` and status `pending_payment`.
- After successful payment, account subscription is extended for 1 month; all mailboxes in that account inherit access (`expires_at` is included in mailbox response).

6. Poll mailbox status:

```bash
curl http://localhost:8080/v1/mailboxes/<mailbox-id> \
  -H 'X-API-Token: <api-token>'
```

7. Resolve IMAP login from mailbox access token:

```bash
curl -X POST http://localhost:8080/v1/imap/resolve \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<mailbox-access-token>"}'
```

- If not paid yet, API returns `409` with `{ "status": "waiting_payment" }`.

8. Fetch unread mails via API endpoint:

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<mailbox-access-token>","unread_only":true,"limit":20,"include_body":false}'
```

- `unread_only` defaults to `true`.
- `include_body` defaults to `false` for list endpoint.

9. Fetch a single message by UID:

```bash
curl -X POST http://localhost:8080/v1/imap/messages/get \
  -H 'Content-Type: application/json' \
  -d '{"access_token":"<mailbox-access-token>","uid":1,"include_body":true}'
```

- `include_body` defaults to `true` for get-by-uid endpoint.

Human-only fallback:
- If both tokens are lost, owner can run email recovery endpoints:
  - `POST /v1/accounts/recovery/start`
  - `POST /v1/accounts/recovery/complete`

Recovery for humans uses URL with token included:
- Email contains link like `/v1/accounts/recovery/complete?token=<one-time-token>`
- Opening link returns plain text credentials for manual re-linking.
