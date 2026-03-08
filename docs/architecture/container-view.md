# Container View

## Main Containers

| Container | Technology | Responsibility |
| --- | --- | --- |
| HTTP API | Go, `net/http` | Accepts mailbox claim, access resolve, legacy account APIs, payment callbacks, and health checks. |
| Core services | Go packages in `internal/core/service` | Enforces mailbox, payment, account, and access rules. |
| Domain and ports | Go packages in `internal/domain` and `internal/core/ports` | Defines business entities and adapter seams. |
| Repository adapters | GORM + SQLite | Persist mailboxes, accounts, recovery state, and refresh tokens. |
| Payment adapters | Polar, Stripe, mock | Create payment sessions and validate completion state. |
| Notification adapters | Resend, SendGrid, log | Deliver payment and recovery notifications. |
| Identity adapter | `edproof` verifier adapter | Verifies key proof and derives a stable key fingerprint. |
| Mail runtime provisioner | GORM-backed adapter | Writes mailbox runtime records used by the receive-only mail stack. |
| Message reader | IMAP adapter | Reads mailbox contents for future inbound-read APIs. |
| SQLite database | SQLite | Stores durable service state. |

## Code Mapping

| Area | Paths |
| --- | --- |
| Entry point | `cmd/app/main.go` |
| HTTP adapter | `internal/adapters/httpapi` |
| Core services | `internal/core/service` |
| Ports | `internal/core/ports` |
| Domain | `internal/domain` |
| Repository adapters | `internal/adapters/repository` |
| Payment adapters | `internal/adapters/payment` |
| Notification adapters | `internal/adapters/notify` |
| Identity adapter | `internal/adapters/identity/edproof` |
| Platform config and DB | `internal/platform/config`, `internal/platform/database` |

## Important Flows

### Key-bound claim

1. HTTP handler accepts `billing_email` and key proof.
2. Identity adapter verifies the proof.
3. Mailbox service reuses an existing mailbox by key fingerprint or creates a new one.
4. Payment adapter creates a checkout session.
5. Notification adapter sends the payment link.

### Payment activation

1. Payment callback reaches the HTTP adapter.
2. Payment adapter verifies the checkout state with the provider.
3. Mailbox service marks the mailbox paid and active.
4. Provisioner ensures the runtime mailbox exists.

### Access resolve

1. HTTP handler accepts protocol plus key proof.
2. Identity adapter verifies the proof.
3. Mailbox service looks up the mailbox by key fingerprint.
4. If the mailbox is active, the service returns IMAP access details.
