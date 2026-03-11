# Session: Smoketest Review Fixes and Deploy Pipeline Repair

**Date:** 2026-03-11
**Branch:** `task/review-smoketest-fixes`, `task/fix-deploy-missing-edproof`

---

## Context

Ran `/start:review smoketests we have` — comprehensive code review of all test infrastructure in the mailservice project.
Agent Team mode with 4 parallel reviewers (Go quality, security, constitution compliance, shell robustness).

## Review Results

Synthesized report: 3 CRITICALs, 10 HIGHs, 10 MEDIUMs, 3 LOWs.
Verdict: APPROVE WITH COMMENTS.
User chose: Fix CRITICALs + HIGHs.

## Fixes Applied

### CRITICALs

- **C1:** Removed `t.Parallel()` from repository tests — goose global state (`SetBaseFS`/`SetDialect`) causes data races.
- **C2:** Replaced 13 sentinel error comparisons (`!=`) with `errors.Is()` across `challenge_test.go`, `mailbox_service_test.go`, `account_service_test.go`.
- **C3:** Added `--max-time 30` to curl calls in `smoke-test-mailbox.sh`.

### HIGHs

- **H1:** Strip IMAP LOGIN line from failure output to prevent credential leakage in CI logs.
- **H2:** Replace heredoc with printf for env file generation in `deploy-smoke.yml` (prevent shell expansion of secrets).
- **H3:** Stage env file in `/root/.mailservice-staging` instead of `/tmp`.
- **H4:** Check `json.Unmarshal` errors in 5 handler test sites.
- **H5:** Added domain-level tests: `Mailbox.Usable()` (7 cases), `Account.SubscriptionActive()` (4 cases).
- **H6:** Added unconfigured-secret guard tests for Stripe and Polar webhooks.
- **H8:** Consolidated 3 near-identical HTTP helper functions into one unified `http_json` with thin wrappers.
- **H9:** Added timeout detection (timeout/gtimeout) for openssl s_client.
- **H10:** Quoted IMAP credentials in LOGIN command.

### Commit

`e4e612e` — fix: address CRITICAL and HIGH findings from smoketest review (274 insertions, 97 deletions, 10 files)

## Deploy Failure and Fix

After merging and pushing, smoke deploy failed.
Root cause: `EDPROOF_HMAC_SECRET` was never in the smoke deploy pipeline (production had it, smoke didn't).
The service crashed in 14ms on startup — `config.Load()` requires this secret.

Previous deploys likely succeeded via a manually-placed `.env` file at `/var/lib/mailservice/.env` (the app's `loadDotEnv()` reads from working directory as fallback).

### Fix

1. Added `EDPROOF_HMAC_SECRET` to `deploy-smoke.yml` (env file generation, env mapping, validation check).
2. Generated and set the GitHub secret: `gh secret set EDPROOF_HMAC_SECRET --env smoke`.
3. Redeployed — all green.

Commit: `544dcf6` — fix(ci): add missing EDPROOF_HMAC_SECRET to smoke deploy

## Workflow Validation

Ran `actionlint` on both `deploy-smoke.yml` and `deploy-production.yml`.
Result: clean — only SC2129 style hints (cosmetic, no functional issues).

## Remaining Follow-ups

- **H6 full:** Stripe webhook test with actual signed payload (needs stripe-go SDK test helpers).
- **H7:** Account/recovery HTTP handler tests (needs new service fakes — 6 handlers with zero HTTP-level coverage).
- **10 MEDIUMs + 3 LOWs** from the original review.

## Key Learnings

- Smoke deploy env file was missing `EDPROOF_HMAC_SECRET` since the feature was introduced — masked by a manual `.env` on the server.
- The app has a dual config source: systemd `EnvironmentFile` + `godotenv.Read()` from working directory. This fallback behavior hid the CI gap.
- Production `deploy-production.yml` already had the secret; smoke was the gap.
