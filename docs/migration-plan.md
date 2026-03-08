# Migration Plan

## Goal

Move from the legacy account/token mailbox flow to the key-bound mailbox flow
without breaking existing clients during rollout.

## Current State

Preferred flow:
- `POST /v1/mailboxes/claim`
- payment link delivery
- payment activation
- `POST /v1/access/resolve`

Legacy flow still supported:
- `POST /v1/accounts`
- `POST /v1/auth/refresh`
- `POST /v1/accounts/recovery/start`
- `POST /v1/accounts/recovery/complete`
- `POST /v1/mailboxes`
- `POST /v1/imap/resolve`
- `POST /v1/imap/messages`
- `POST /v1/imap/messages/get`

## Compatibility Rules

During migration:
- legacy account/token endpoints remain available
- key-bound endpoints are preferred for new integrations
- documentation must clearly mark legacy paths as transitional
- tests must continue covering unchanged legacy mailbox behavior

## Exit Criteria For Legacy Removal

Do not remove legacy account-centric mailbox auth until all of the following are
true:

1. all first-party integrations use key-bound claim and resolve flows
2. mailbox access no longer depends on account API tokens
3. support expectations for recovery/token-based mailbox auth are explicitly ended
4. payment completion works for the key-bound mailbox path in production
5. operational docs are updated for the final model

## Planned Removal Scope

After migration exit criteria are met, retire:
- account-token mailbox access assumptions
- mailbox access token distribution as the primary integration path
- account refresh-token use for mailbox access
- recovery-based mailbox access assumptions

Candidate endpoints/tables to review for retirement:
- `POST /v1/accounts`
- `POST /v1/auth/refresh`
- `POST /v1/accounts/recovery/start`
- `POST /v1/accounts/recovery/complete`
- `POST /v1/imap/resolve`
- `POST /v1/imap/messages`
- `POST /v1/imap/messages/get`
- `accounts`
- `account_recoveries`
- `refresh_tokens`

## Recommended Rollout Order

1. keep legacy routes stable
2. move new clients to key-bound claim and resolve
3. verify payment activation and operations around the new flow
4. announce deprecation window for legacy mailbox auth
5. remove legacy mailbox auth paths only after the exit criteria above are met
