# OpenClaw flow

Use this flow for bot integration.

1. Create account and get API token (first run only):

```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

2. If account already exists, API responds with `409 { "status": "recovery_required" }`.

3. Recovery (token lost):

Start recovery (always `202`, no account existence leak):

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/start \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com"}'
```

Service sends one-time code to owner email. Bot reads owner mailbox and completes recovery:

```bash
curl -X POST http://localhost:8080/v1/accounts/recovery/complete \
  -H 'Content-Type: application/json' \
  -d '{"owner_email":"owner@example.com","code":"<recovery-code>"}'
```

4. Use returned `api_token` for all bot requests (`X-API-Token` or `Authorization: Bearer`).

5. List existing mailboxes:

```bash
curl http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

6. Create mailbox:

```bash
curl -X POST http://localhost:8080/v1/mailboxes \
  -H 'X-API-Token: <api-token>'
```

- If there is no pending mailbox, service creates one, sends payment link to owner email, and returns `201`.
- If a pending mailbox already exists, service returns it with `200` and status `pending_payment`.

7. Poll mailbox status:

```bash
curl http://localhost:8080/v1/mailboxes/<mailbox-id> \
  -H 'X-API-Token: <api-token>'
```

8. Resolve IMAP login from mailbox access token:

```bash
curl -X POST http://localhost:8080/v1/imap/resolve \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<mailbox-access-token>"}'
```

- If not paid yet, API returns `409` with `{ "status": "waiting_payment" }`.

9. Fetch mails via API proxy endpoint (placeholder):

```bash
curl -X POST http://localhost:8080/v1/imap/messages \
  -H 'Content-Type: application/json' \
  -H 'X-API-Token: <api-token>' \
  -d '{"access_token":"<mailbox-access-token>"}'
```
