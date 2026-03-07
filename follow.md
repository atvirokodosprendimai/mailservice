# OpenClaw flow

Use this flow for bot integration.

1. Start account access by owner email (works for both new registration and token recovery):

```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

- API returns `202` with generic status.
- Service sends one-time code to owner email.
- Rate limit: max 1 code request per owner email per minute (`429` if exceeded).

2. Bot reads owner mailbox and completes access to get API token:

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/complete \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com","code":"<recovery-code>"}'
```

3. Use returned `api_token` for all bot requests (`X-API-Token` or `Authorization: Bearer`).

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
