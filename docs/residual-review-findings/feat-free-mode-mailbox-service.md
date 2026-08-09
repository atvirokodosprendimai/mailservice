# Residual Review Findings — feat/free-mode-mailbox-service

Record of code-review residuals from the free-mode feature branch (ce-code-review run 20260809-151217-065ec050, rebased onto origin/main at 389aed2).

## Filed (tracker tickets)

- P2 — internal/domain/mailbox.go:31 — Replace transient ActivationURL field with explicit claim result — [mailservice-dvz](https://github.com/atvirokodosprendimai/mailservice/issues/mailservice-dvz) — free-mode claim carries the activation URL to the handler/email via a never-persisted field on the domain entity; adapter concern leaks into core/domain.
- P3 — internal/core/service/mailbox_service.go:150 — Rate-limit activation-link regeneration on re-claim — [mailservice-5ud](https://github.com/atvirokodosprendimai/mailservice/issues/mailservice-5ud) — agent retry loops re-email and invalidate the previous activation link on every claim call, with no per-key rate limit.
- P2 — internal/core/service/mailbox_service.go:635 — Snapshot mailbox/account expiry before free-mode switchover — [mailservice-cb5](https://github.com/atvirokodosprendimai/mailservice/issues/mailservice-cb5) — switchover permanently overwrites paid-expiry history; take a DB snapshot before running it.

## Noted (advisory, no ticket)

- P3 — internal/adapters/notify/log_notifier.go:24 — LogNotifier.SendActivationLink prints the raw one-time activation URL; mirrors the existing SendPaymentLink/SendRecoveryLink convention, but tokens should not land in logs outside local dev.
- P3 — internal/platform/config/config.go:196 — FREE_MODE fail-open on unparsable values (defaults to on); consider a warning log so an operator typo is visible.
- P3 — .github/workflows/deploy-production.yml:410 — FREE_MODE absent from both deploy workflow env surfaces; harmless while the default is on, but re-enabling paid mode requires FREE_MODE=false to reach the service in deployed environments.
- P3 — internal/adapters/httpapi/handler.go:1142 — mock pay reports `{"status":"paid"}` in free mode while nothing activates (dev-only surface).
- P2 — internal/core/service/mailbox_service.go — file remains ~1047 lines after extracting the free-mode cluster to mailbox_service_free.go; further splitting is optional.

## Resolved during review

- Polar success page nil-deref in free mode (P1) — fixed, handler test added.
- Legacy account-bound pending mailboxes usable without activation (P1) — fixed, service test added.
- Switchover email-failure orphaning pending mailboxes (P2) — fixed (email sent before token persisted), test added.
- SetPublicBaseURL silently broken activation links (P2) — fixed (explicit error), test added.
- Activation endpoint 200-for-invalid (advisory) — now 404, distinguishable programmatically; docs updated with self-activation path.
- README/follow.md/AGENTS.md flow docs still describing payment — updated to activation flow.
