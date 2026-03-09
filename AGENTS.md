# AGENTS.md

Practical guidance for coding agents working in this repository.

## Project Snapshot
- Go API for paid inbound mailbox provisioning (OpenClaw use case).
- Preferred flow: `POST /v1/mailboxes/claim` -> pay -> `POST /v1/access/resolve`.
- Legacy account/token flow still exists and must stay stable during migration.
- Product boundary: inbound email + IMAP read access only; no SMTP/outbound sending.

## Architecture (Hexagonal)
- Domain models: `internal/domain`
- Ports/interfaces: `internal/core/ports`
- Core business services: `internal/core/service`
- Adapters:
  - HTTP API: `internal/adapters/httpapi`
  - Repositories (GORM): `internal/adapters/repository`
  - Payment providers: `internal/adapters/payment`
  - Notifiers: `internal/adapters/notify`
  - Token generation: `internal/adapters/token`
  - IMAP reader + key verification: `internal/adapters/imap`, `internal/adapters/identity`
- Platform:
  - Config loader: `internal/platform/config`
  - DB + migrations: `internal/platform/database`
- Entrypoint wiring only: `cmd/app/main.go`

Rule of thumb: core packages depend on ports/domain, never on concrete adapters.

## Build, Run, Test, Lint
No Makefile wrapper in this repo; use direct commands.

### Run and build
- Run app: `go run ./cmd/app`
- Build app: `go build ./cmd/app`
- Build all packages: `go build ./...`

### Tests
- Run all tests: `go test ./...`
- Verbose tests: `go test -v ./...`
- Disable test cache while iterating: `go test -count=1 ./...`

Single-test workflows (important):
- Run one package: `go test ./internal/core/service`
- Run one exact test name:
  `go test ./internal/adapters/httpapi -run '^TestHandleClaimMailboxCreatesPendingMailbox$'`
- Run one exact test in another package:
  `go test ./internal/platform/config -run '^TestLoad$'`
- Run subtests:
  `go test ./path/to/pkg -run 'TestName/subcase'`

Coverage and race checks:
- Coverage summary: `go test ./... -cover`
- Detailed package coverage:
  `go test ./internal/core/service -coverprofile=cover.out && go tool cover -func=cover.out`
- Race detector (slower): `go test -race ./...`

### Formatting and lint-like checks
- Format changed files: `gofmt -w <files>`
- Format all tracked Go files: `gofmt -w $(git ls-files '*.go')`
- Static checks: `go vet ./...`

Notes:
- No committed `golangci-lint` config at the moment.
- CI is deployment-focused; run `go test ./...` locally before push.

### Infra/OpenTofu checks (when touching infra)
If changing `infra/opentofu/**` or related workflows, run:
- `tofu fmt -check infra/opentofu`
- `tofu init -backend=false infra/opentofu`
- `tofu validate infra/opentofu`
- `go test ./...`

Reference: `docs/local-workflow-validation.md`.

## Data and Persistence
- DB: SQLite (pure Go driver `github.com/glebarez/sqlite`, no cgo)
- ORM: GORM (`gorm.io/gorm`)
- Migrations: goose SQL migrations embedded and applied at startup
- Core tables: `accounts`, `mailboxes`, `account_recoveries`, `refresh_tokens`

## API/Auth Behaviors To Preserve
- Refresh tokens are one-time use and stored hashed.
- Recovery code TTL is 10 minutes.
- Recovery start is rate-limited per account (1 request/minute).
- Global semaphore can reject with `503` + `Retry-After` + `retry_after_seconds`.

## Code Style Guidelines
Follow existing Go style and package patterns.

### Imports
- Let `gofmt` order imports (stdlib, external, internal).
- Keep imports minimal; remove unused imports.
- Avoid aliases unless needed for name collision clarity.

### Formatting
- Always run `gofmt -w` on modified Go files.
- Keep functions readable; prefer helpers over deep nesting.
- Add comments only when intent is non-obvious.

### Types and APIs
- `context.Context` is first arg for I/O/service/repo methods.
- Keep interfaces in `internal/core/ports`; adapter implementations stay in adapters.
- HTTP request/response payloads should use explicit structs + JSON tags.
- Export only what must be used across packages.

### Naming
- Exported identifiers: `PascalCase`; unexported: `camelCase`.
- Keep acronym style consistent (`IMAP`, `API`, `ID`, `URL`).
- Sentinel errors use `Err...` names in `ports` for cross-layer matching.
- Prefer domain-clear names (`Mailbox`, `PaymentSession`, `SubscriptionExpiresAt`).

### Error handling
- Do not `panic` in normal flow; return errors.
- Wrap contextual failures with `%w` (especially service/repository layers).
- Use `errors.Is` for branching on sentinel errors.
- Keep HTTP error payload stable: `{"error":"..."}` via `writeError`.
- Map errors to status codes deliberately; do not leak internals.

### HTTP conventions
- Decode JSON strictly (`decodeJSON` with unknown fields disallowed).
- Prefer explicit request structs over dynamic maps.
- Return JSON for API endpoints and preserve status code semantics.
- Preserve backward compatibility unless task explicitly changes contract.

### Persistence conventions
- Repositories translate between GORM models and domain entities.
- Convert `gorm.ErrRecordNotFound` to port sentinel errors.
- Normalize user input where existing code does (trim/lowercase emails, fingerprints).

### Testing conventions
- Add or update tests with behavior changes (TDD preferred).
- Tests use stdlib `testing` + manual fakes (no heavy mocking frameworks).
- Prefer deterministic tests with in-memory fakes or temp SQLite DBs.
- During development use targeted `-run`, then finish with `go test ./...`.

## Security and Secrets
- Never commit `.env` or production secrets.
- Use `.env.example` only as a variable reference.
- Keep webhook verification behavior intact when editing payment webhooks.

## Agent Workflow Guardrails
- Preserve hexagonal boundaries; no adapter concerns in core/domain code.
- Keep API responses backward compatible unless explicitly requested.
- Avoid introducing cgo dependencies.
- Prefer minimal, surgical diffs over broad refactors.
- This project uses the `tk` CLI ticket system for task management; run `tk help` when you need to use it.
- If flows/docs change, update docs (`README.md`, `follow.md`, architecture docs).
- Before merge, check CI plus review comments/unresolved threads.

## Cursor/Copilot Rules Check
Checked repository for agent instruction files:
- `.cursor/rules/`: not present
- `.cursorrules`: not present
- `.github/copilot-instructions.md`: not present

If these files appear later, treat them as additional mandatory constraints.
