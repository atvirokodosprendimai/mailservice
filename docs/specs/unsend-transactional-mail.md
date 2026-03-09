# Unsend Transactional Mail Integration Spec

Ticket: `mai-wvcw`

## Phase 1: Requirements

### User stories

- As an operator, I want transactional emails to be sent through hosted Unsend so production mail delivery is reliable.
- As an operator, I want mail delivery to fall back to existing notifier behavior when Unsend is not configured.
- As a maintainer, I want clear, testable notifier selection rules so behavior is predictable across environments.

### Acceptance criteria (EARS)

1. WHEN `UNSEND_KEY` is configured THEN system SHALL initialize Unsend notifier for transactional email delivery.
2. WHEN Unsend notifier is active AND an email send is requested THEN system SHALL call `https://unsend.admin.lt/` using authenticated API request.
3. IF `UNSEND_KEY` is not configured THEN system SHALL keep current notifier selection behavior unchanged.
4. WHEN Unsend API returns an error THEN system SHALL return an email-send error to caller and log contextual failure details.
5. WHEN operator runs tests THEN system SHALL include notifier wiring tests and Unsend adapter request/response tests.
6. WHEN documentation is updated THEN system SHALL describe required `UNSEND_KEY` configuration and notifier precedence.

### Edge cases

- Invalid/missing recipient email payload from caller.
- Non-2xx Unsend HTTP responses.
- Network timeout/TLS/connectivity failures.
- Empty sender configuration values.

## Phase 2: Design

### Overview

Add a new `notify` adapter for Unsend and wire it into notifier selection in `cmd/app/main.go` with clear precedence.

### Architecture

- `internal/adapters/notify/unsend.go`: Unsend HTTP notifier implementation.
- `internal/platform/config/config.go`: load `UNSEND_KEY` and optional sender metadata vars.
- `cmd/app/main.go`: select Unsend notifier when configured.

### Components and interfaces

- Reuse existing `ports.Notifier` interface.
- New constructor: `notify.NewUnsendNotifier(baseURL, apiKey, fromEmail, fromName)`.
- Adapter sends transactional message payload to Unsend API endpoint.

### Data model

- Input: `ownerEmail`, email subject/body, recovery/payment URL data (existing notifier call paths).
- Config:
  - `UNSEND_KEY` (required to enable Unsend)
  - `UNSEND_BASE_URL` (default `https://unsend.admin.lt`)
  - optional sender defaults if needed by API contract.

### Error handling

- Wrap HTTP failures with `%w` and operation context.
- Treat non-2xx as send failure.
- Keep caller-facing behavior consistent with existing notifier error propagation.

### Testing strategy

- Adapter tests using `httptest.Server` for success and failure paths.
- App wiring tests for notifier selection precedence.
- Regression test ensuring legacy notifier behavior remains when Unsend is unconfigured.

## Phase 3: Task plan

- [x] 1. Add config support for Unsend
  - Add config fields/env loading for `UNSEND_KEY` and `UNSEND_BASE_URL`.
  - _Requirements: 1, 3, 6_

- [x] 2. Implement Unsend notifier adapter
  - Add `internal/adapters/notify/unsend.go` implementing `ports.Notifier`.
  - Add adapter unit tests for 2xx, non-2xx, and timeout/failure.
  - _Requirements: 2, 4, 5_

- [x] 3. Wire notifier selection in app bootstrap
  - Update `cmd/app/main.go` notifier selection precedence and logging.
  - Preserve fallback path to existing providers when Unsend absent.
  - _Requirements: 1, 3_

- [x] 4. Update docs and examples
  - Update `.env.example` and `README.md` for Unsend config.
  - _Requirements: 6_

- [x] 5. Validate end-to-end behavior
  - Run targeted tests and `go test ./...`.
  - _Requirements: 5_
