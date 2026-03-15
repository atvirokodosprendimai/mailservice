---
tldr: Handle subscription cancellation proactively — webhook handler + periodic sweep
category: ops
---

# Todo: Proactive mailbox expiry instead of lazy check on access

Currently mailboxes only expire when someone tries to use them (`validateMailboxSubscription` in `mailbox_service.go:325`).
If nobody logs in, the mailbox stays "active" in the DB indefinitely.

## What's needed

1. **Polar webhook handler for subscription lifecycle events** — `subscription.canceled`, `subscription.revoked`, `subscription.updated` with status changes. Immediately set mailbox status to `expired` when subscription ends.
2. **Periodic sweep** — cron or background goroutine that checks all active mailboxes against their `ExpiresAt` and flips expired ones. Catches edge cases where webhooks are missed.
3. **Cleanup** — optionally remove Dovecot/Postfix mail user entries for long-expired mailboxes to free disk.

## Related files

- Lazy expiry check: `internal/core/service/mailbox_service.go:325-357`
- Polar webhook handler: `internal/adapters/httpapi/handler.go:954`
- Domain model: `internal/domain/mailbox.go` (`ExpiresAt`, `Usable()`)
- Mail provisioner: `internal/adapters/repository/mail_runtime.go`
