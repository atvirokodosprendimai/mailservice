---
title: "feat: HTTP caching hints (ETag, Last-Modified)"
type: feat
status: active
date: 2026-03-11
---

# HTTP Caching Hints (ETag, Last-Modified)

## Overview

Add HTTP conditional-request support to the two GET endpoints that return cacheable data:
`GET /v1/mailboxes` and `GET /v1/mailboxes/{id}`. Clients can then use `If-None-Match` /
`If-Modified-Since` to avoid re-downloading unchanged responses, saving bandwidth and
reducing perceived latency.

All POST endpoints are naturally uncacheable and need no changes.

## Problem Statement

Every API response today is served without caching hints. A client polling
`GET /v1/mailboxes/{id}` to check mailbox status always gets a full 200 response, even
when nothing has changed. For agents that poll after payment, this is wasteful —
especially on slow connections or metered infrastructure.

## Proposed Solution

### Scope

| Endpoint | Caching Strategy |
|---|---|
| `GET /v1/mailboxes/{id}` | `ETag` (strong, from `id+UpdatedAt`), `Last-Modified` from `UpdatedAt`, respond `304` on `If-None-Match` / `If-Modified-Since` match, `Cache-Control: private, no-cache` |
| `GET /v1/mailboxes` | `ETag` (weak, from `max(UpdatedAt)` across list), respond `304` on match, `Cache-Control: private, no-cache` |
| `GET /healthz` | `Cache-Control: public, max-age=5` (no ETag needed) |
| `GET /` (homepage) | Keep `no-store` (already set) |
| All POST endpoints | No changes (POST is uncacheable by spec) |

`Cache-Control: private, no-cache` means: response is user-specific (behind auth), and the
client must revalidate with the server before using a cached copy. This is the correct
semantic for authenticated resources that support conditional requests.

### ETag Generation

**Single mailbox** (`GET /v1/mailboxes/{id}`):

```go
etag := fmt.Sprintf(`"%s-%d"`, mailbox.ID, mailbox.UpdatedAt.UnixNano())
```

Strong ETag — the response is byte-identical for the same ID+UpdatedAt.

**Mailbox list** (`GET /v1/mailboxes`):

```go
maxUpdated := maxUpdatedAt(mailboxes)
etag := fmt.Sprintf(`W/"%s-%d"`, account.ID, maxUpdated.UnixNano())
```

Weak ETag — the list might differ in ordering details but is semantically equivalent.

### Conditional Request Handling

Before executing the mailbox query (after auth), check request headers:

```go
func checkNotModified(r *http.Request, etag string, lastModified time.Time) bool {
    // If-None-Match takes precedence per RFC 7232 §3.3
    if match := r.Header.Get("If-None-Match"); match != "" {
        return match == etag || match == "*"
    }
    if ims := r.Header.Get("If-Modified-Since"); ims != "" {
        t, err := http.ParseTime(ims)
        if err == nil && !lastModified.After(t) {
            return true
        }
    }
    return false
}
```

When matched, return `304 Not Modified` with the same ETag (no body).

**Note:** The auth middleware (`withAccountToken`) always runs first — a 304 still requires
valid authentication. The saving is skipping the mailbox DB query and response serialization,
not the auth check.

### Side-Effect Awareness

`validateMailboxSubscription` can mutate mailbox status during reads (marking expired
subscriptions). This naturally invalidates the ETag via the updated `UpdatedAt` timestamp,
so it's safe — a stale client simply won't get a 304 after the expiry mutation.

## Acceptance Criteria

- [ ] `GET /v1/mailboxes/{id}` returns `ETag` and `Last-Modified` headers
- [ ] `GET /v1/mailboxes/{id}` returns `304 Not Modified` when `If-None-Match` matches
- [ ] `GET /v1/mailboxes/{id}` returns `304 Not Modified` when `If-Modified-Since` is not before `UpdatedAt`
- [ ] `GET /v1/mailboxes` returns a weak `ETag` header
- [ ] `GET /v1/mailboxes` returns `304` when `If-None-Match` matches
- [ ] Both endpoints set `Cache-Control: private, no-cache`
- [ ] `GET /healthz` sets `Cache-Control: public, max-age=5`
- [ ] All POST endpoints remain unchanged (no caching headers)
- [ ] Tests cover: 200 with ETag, 304 on matching ETag, 200 on stale ETag, 304 on matching If-Modified-Since
- [ ] Homepage retains `no-store`

## Implementation

### Phase 1: Core caching helpers

File: `internal/adapters/httpapi/cache.go`

- [ ] `generateETag(id string, updatedAt time.Time) string` — strong ETag for single resource
- [ ] `generateWeakETag(scope string, updatedAt time.Time) string` — weak ETag for collections
- [ ] `checkNotModified(r *http.Request, etag string, lastModified time.Time) bool`
- [ ] `writeNotModified(w http.ResponseWriter, etag string)` — writes 304 with ETag header
- [ ] `setCacheHeaders(w http.ResponseWriter, etag string, lastModified time.Time)` — sets ETag, Last-Modified, Cache-Control

### Phase 2: Apply to handlers

File: `internal/adapters/httpapi/handler.go`

- [ ] `handleGetMailbox`: after fetching mailbox, compute ETag, check conditional, return 304 or 200 with headers
- [ ] `handleListMailboxes`: after fetching list, compute weak ETag from max UpdatedAt, check conditional, return 304 or 200
- [ ] `handleHealth`: add `Cache-Control: public, max-age=5`

### Phase 3: Tests

File: `internal/adapters/httpapi/handler_test.go`

- [ ] Test ETag present in `GET /v1/mailboxes/{id}` response
- [ ] Test 304 when `If-None-Match` matches current ETag
- [ ] Test 200 when `If-None-Match` doesn't match (stale)
- [ ] Test 304 when `If-Modified-Since` is after `UpdatedAt`
- [ ] Test 200 when `If-Modified-Since` is before `UpdatedAt`
- [ ] Test weak ETag on `GET /v1/mailboxes` list
- [ ] Test healthz has `Cache-Control: public, max-age=5`

## Sources & References

### Internal References

- Response writing: `internal/adapters/httpapi/handler.go:1184-1192` (writeJSON, writeError)
- Middleware pattern: `internal/adapters/httpapi/semaphore.go` (withGlobalSemaphore)
- Auth middleware: `internal/adapters/httpapi/handler.go:1081-1100` (withAccountToken)
- Domain model: `internal/domain/mailbox.go` (Mailbox.UpdatedAt)
- Subscription validation side-effect: `internal/core/service/mailbox_service.go` (validateMailboxSubscription)
- Existing cache headers: `internal/adapters/httpapi/handler.go:119-121` (homepage no-store)
