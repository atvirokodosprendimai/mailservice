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

<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Dolt-powered version control with native sync
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update <id> --claim --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task atomically**: `bd update <id> --claim`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs via Dolt:

- Each write auto-commits to Dolt history
- Use `bd dolt push`/`bd dolt pull` for remote sync
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

<!-- END BEADS INTEGRATION -->
