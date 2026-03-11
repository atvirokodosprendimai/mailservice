# Project Constitution

> Version: 1.0.0 | Last Updated: 2026-03-11
> Project Type: single-app (Go mailservice, hexagonal architecture)

## Security

### SEC-001 No Hardcoded Secrets

```yaml
level: L1
check: All secrets (API keys, tokens, passwords, HMAC keys) loaded from environment variables via os.Getenv(). No hardcoded secrets in source code.
scope: "**/*.go"
exclude: "**/*_test.go"
message: Hardcoded secret detected. Use environment variables.
```

Secrets validated at startup in `config.Load()`. EDPROOF_HMAC_SECRET must be >= 32 bytes.

### SEC-002 Constant-Time Secret Comparison

```yaml
level: L1
pattern: "[^.]\\b(apiToken|secret|key|token|hmac)\\b.*==\\s"
scope: "**/*.go"
exclude: "**/*_test.go"
message: Secret comparison must use crypto/subtle.ConstantTimeCompare(), never ==.
```

Prevents timing attacks on API keys, admin tokens, and HMAC values.

### SEC-003 Request Body Size Limits

```yaml
level: L1
check: All HTTP request body reads (JSON decode, io.ReadAll) must be wrapped with io.LimitReader(r.Body, maxRequestBodyBytes)
scope: "internal/adapters/httpapi/**/*.go"
exclude: "**/*_test.go"
message: Unbounded request body read. Wrap with io.LimitReader.
```

Hard cap of 1 MB (`1 << 20`) on all request bodies to prevent memory exhaustion.

### SEC-004 Strict JSON Decoding

```yaml
level: L1
check: JSON decoders must call DisallowUnknownFields() and reject trailing content
scope: "internal/adapters/httpapi/**/*.go"
exclude: "**/*_test.go"
message: JSON decoder missing DisallowUnknownFields() or trailing content check.
```

Prevents field injection and malformed payloads.

### SEC-005 Webhook Signature Verification

```yaml
level: L1
check: All webhook handlers must verify signatures before processing payloads. Polar uses HMAC with timestamp freshness. Stripe uses official SDK ConstructEvent().
scope: "internal/adapters/httpapi/**/*.go"
exclude: "**/*_test.go"
message: Webhook payload processed without signature verification.
```

Timestamp freshness enforced (5-minute window for Polar). Unknown event types ignored with 202 Accepted.

### SEC-006 HTML Output Escaping

```yaml
level: L1
check: All user-controlled values interpolated into HTML must be escaped with html.EscapeString()
scope: "internal/adapters/**/*.go"
exclude: "**/*_test.go"
message: User-controlled value in HTML output without html.EscapeString().
```

Applies to handler responses and email body generation (mailgun, resend, unsend notifiers).

### SEC-007 Parameterized Database Queries

```yaml
level: L1
pattern: "Exec\\s*\\(\\s*fmt\\.Sprintf|Exec\\s*\\([^)]*\\+\\s"
scope: "**/*.go"
exclude: "**/*_test.go"
message: SQL injection risk. Use parameterized queries (? placeholders) or GORM methods.
```

All queries use GORM methods or `?` placeholders. Raw SQL only in embedded migrations.

### SEC-008 No Information Leakage in Errors

```yaml
level: L1
check: Error responses must not reveal resource existence or internal state. Recovery endpoints return generic "email_sent_if_exists". Auth failures return 401/404, never 403 with details.
scope: "internal/adapters/httpapi/**/*.go"
exclude: "**/*_test.go"
message: Error response may leak sensitive information.
```

Prevents account enumeration and auth state disclosure.

## Architecture

### ARCH-001 Core Never Imports Adapters

```yaml
level: L1
check: Files in internal/core/ and internal/domain/ must never import from internal/adapters/ or internal/platform/. All external dependencies flow through ports.go interfaces.
scope: "internal/core/**/*.go, internal/domain/**/*.go"
exclude: "**/*_test.go"
message: Core/domain layer importing from adapters or platform. Use ports interface instead.
```

Hexagonal architecture: core depends on abstractions (ports), never on implementations (adapters).

### ARCH-002 Unidirectional Import Flow

```yaml
level: L1
check: "Import direction enforced: adapters -> ports+domain, services -> ports+domain, domain -> stdlib only. No adapter-to-adapter imports. No circular imports. Only main.go imports adapters and services together."
scope: "internal/**/*.go"
exclude: "**/*_test.go"
message: Import direction violation. Check dependency flow.
```

| From | May Import |
|------|-----------|
| `domain/` | stdlib only |
| `core/ports/` | stdlib, `domain` |
| `core/service/` | stdlib, external libs, `ports`, `domain` |
| `adapters/*` | stdlib, external libs, `ports`, `domain` |
| `platform/*` | stdlib, external libs (never core or adapters) |
| `cmd/app/main.go` | everything (composition root) |

### ARCH-003 Constructor-Based Dependency Injection

```yaml
level: L1
check: All services and adapters use constructor injection via New*() functions. main.go is the single composition root. No global singletons, no service locators, no init() functions for wiring.
scope: "internal/**/*.go"
exclude: "**/*_test.go"
message: Dependency not injected via constructor. Use composition root in main.go.
```

Dependencies explicitly declared in constructor parameters, wired in `main.go:25-105`.

### ARCH-004 One Adapter Per Port

```yaml
level: L1
check: Each adapter struct implements exactly one port interface. Adapters may depend on external SDKs but never on other adapter packages.
scope: "internal/adapters/**/*.go"
exclude: "**/*_test.go"
message: Adapter importing from another adapter package or implementing multiple ports.
```

Clean boundaries: `notify/` implements `Notifier`, `payment/` implements `PaymentGateway`, etc.

### ARCH-005 Config Load Once at Startup

```yaml
level: L2
check: Configuration loaded once in main.go via config.Load() before creating any services. Required fields validated at startup (fail fast). Config values passed to constructors, not entire Config struct.
scope: "cmd/app/main.go, internal/platform/config/**/*.go"
message: Config accessed outside startup or entire Config struct passed to service.
```

Explicit configuration preferred over implicit cascades.

## Code Quality

### QUAL-001 Error Wrapping with Context

```yaml
level: L1
check: Errors wrapped with fmt.Errorf("context: %w", err) preserving error chain. Sentinel errors (ErrMailboxNotFound, etc.) defined only in ports package via var() blocks. Use errors.Is() for sentinel comparison.
scope: "**/*.go"
exclude: "**/*_test.go"
message: Error not wrapped with context or sentinel error defined outside ports package.
```

Provides clear error traces: `"create payment link: generate checkout URL: ..."`.

### QUAL-002 Context as First Parameter

```yaml
level: L1
check: All functions performing I/O (database, HTTP, IMAP, RPC) accept context.Context as the first parameter. Context propagated through entire call chain. Never create context in service layer.
scope: "**/*.go"
exclude: "**/*_test.go"
message: I/O function missing context.Context as first parameter.
```

Enables per-request cancellation, deadlines, and tracing.

### QUAL-003 Validated Constructors

```yaml
level: L1
check: "All exported types use New<Type>() constructors that: (1) validate inputs, (2) apply defaults for empty/zero values, (3) return (T, error) if validation can fail. All fields initialized in constructor."
scope: "**/*.go"
exclude: "**/*_test.go"
message: Exported type missing New*() constructor or constructor missing validation.
```

No lazy initialization. Normalize strings (trim, lowercase) during construction.

### QUAL-004 Named Constants

```yaml
level: L1
check: Timeouts, TTLs, size limits, and thresholds must be assigned to named const or var declarations — not used as inline literals in function bodies. Named constant definitions (e.g., const maxAge = 30 * time.Second) are compliant; the literal is the definition, not a magic number.
scope: "**/*.go"
exclude: "**/*_test.go, **/config.go"
message: Magic number for timeout/limit/TTL. Extract to named const with comment.
```

All timeouts, limits, and TTLs in named `const` blocks (e.g., `recoveryTTL = 10 * time.Minute`).

## Testing

### TEST-001 Standard Library Testing Only

```yaml
level: L1
pattern: "\"github\\.com/stretchr/testify|\"github\\.com/onsi/gomega|\"github\\.com/onsi/ginkgo"
scope: "**/*_test.go"
message: External test framework detected. Use standard testing package with if/t.Fatalf assertions.
```

No assertion libraries. Manual `if condition { t.Fatalf(...) }` pattern only.

### TEST-002 Interface-Based Fakes

```yaml
level: L1
check: Test doubles (fakes/stubs) implement port interfaces from ports package. Defined as unexported types within _test.go files. No mocking libraries (testify/mock, mockgen, gomock).
scope: "**/*_test.go"
message: Mock library detected or fake not implementing port interface.
```

Type-safe, compiler-verified. Fakes are minimal, implementing only needed methods.

### TEST-003 HTTP Testing with httptest

```yaml
level: L1
check: HTTP testing uses net/http/httptest package. NewServer() for external API mocks, NewRequest()/NewRecorder() for handler testing. No external HTTP mocking libraries.
scope: "**/*_test.go"
message: HTTP testing without httptest. Use httptest.NewServer or httptest.NewRequest.
```

Standard library HTTP testing across all adapter and handler tests.

## Dependencies

### DEPS-001 No CGO

```yaml
level: L1
check: All build targets set CGO_ENABLED=0. No C library dependencies. Static binaries only. Verify in Dockerfile, flake.nix, and CI.
scope: "Dockerfile, flake.nix, .github/workflows/**/*.yml"
message: CGO_ENABLED not set to 0 or C dependency detected.
```

Enables static compilation, Alpine containers, and cross-platform builds.

### DEPS-002 Exact Version Pinning

```yaml
level: L1
check: go.mod uses exact semantic versions (X.Y.Z). go.sum checked into VCS. Nix vendorHash updated when dependencies change.
scope: "go.mod, go.sum, flake.nix"
message: Loose version constraint or missing go.sum/vendorHash.
```

Reproducible builds across Docker, Nix, and local development.

### DEPS-003 Minimal Dependencies

```yaml
level: L1
check: No web frameworks (gin, echo, chi) — stdlib net/http only. No logging libraries (logrus, zap, zerolog) — stdlib log only. New direct dependencies require justification.
scope: "go.mod"
message: Prohibited dependency detected. Use stdlib equivalent.
```

Currently 8 direct dependencies. Each serves a specific, justified purpose.

## Performance

### PERF-001 Global Concurrency Ceiling

```yaml
level: L1
check: Global request semaphore limits concurrent requests (MaxConcurrentReqs, default 100). Exceeding capacity returns 503 with randomized Retry-After header (3-100s).
scope: "internal/adapters/httpapi/**/*.go"
message: Request handler bypasses global semaphore.
```

Prevents unbounded goroutine proliferation under load.

### PERF-002 External API Timeouts

```yaml
level: L1
check: All http.Client instances set explicit Timeout (10s standard). No default (infinite timeout) clients. Store client in struct for reuse.
scope: "internal/adapters/**/*.go"
exclude: "**/*_test.go"
message: http.Client without explicit Timeout. Set Timeout in constructor.
```

Prevents cascading failures from slow/hung upstream services.

### PERF-003 Graceful Shutdown

```yaml
level: L1
check: Server handles SIGTERM/SIGINT with graceful shutdown deadline (10s). All spawned goroutines tracked and joined via channels. No fire-and-forget goroutines.
scope: "cmd/app/main.go"
message: Missing graceful shutdown or untracked goroutine.
```

Currently only 2 goroutine spawn sites, both explicitly synchronized.

### PERF-004 Defer Resource Cleanup

```yaml
level: L1
check: All external resources (HTTP response bodies, IMAP connections, semaphore tokens, file handles) freed via defer blocks immediately after acquisition.
scope: "**/*.go"
exclude: "**/*_test.go"
message: Resource acquired without deferred cleanup.
```

Guarantees cleanup even on error paths.

### PERF-005 Query Pagination Limits

```yaml
level: L2
check: All list/fetch operations enforce maximum result count (100 items) with sensible default (20). Prevents unbounded queries to external systems (IMAP).
scope: "internal/core/service/**/*.go"
message: List operation without pagination limit enforcement.
```

Hard ceiling prevents memory exhaustion from oversized responses.

## Workflow

### WORK-001 Push After Merge

```yaml
level: L1
check: After merging a task branch to main, push to origin immediately. Code that only exists locally is not shipped — it has zero value until it reaches production.
scope: "git workflow"
message: Merged to main but not pushed. Push immediately.
```

If it's not on production, we wasted time and resources.

## Custom Rules

<!-- Project-specific rules can be added here -->
