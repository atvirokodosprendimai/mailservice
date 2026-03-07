# OpenClaw flow

Use this flow for bot integration.

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
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<mailbox-access-token>"}'
```

- If not paid yet, API returns `409` with `{ "status": "waiting_payment" }`.

8. Fetch mails via API proxy endpoint (placeholder):

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<mailbox-access-token>"}'
```

Human-only fallback:
- If both tokens are lost, owner can run email recovery endpoints:
  - `POST /v1/accounts/recovery/start`
  - `POST /v1/accounts/recovery/complete`

Recovery for humans uses URL with token included:
- Email contains link like `/v1/accounts/recovery/complete?token=<one-time-token>`
- Opening link returns plain text credentials for manual re-linking.
