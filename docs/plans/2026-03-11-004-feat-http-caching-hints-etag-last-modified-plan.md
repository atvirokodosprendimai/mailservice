---
title: "feat: ETag on homepage and agent skill doc"
type: feat
status: active
date: 2026-03-11
---

# ETag on Homepage and Agent Skill Document

## Overview

Add an ETag (based on the build number) to the homepage (`GET /`) and serve the agent API
skill document at `GET /v1/docs/agent-skill` with the same ETag. This lets an agent check
whether it already has the latest version of the instructions without re-downloading them.

## Problem Statement

An agent that has previously loaded the API instructions has no way to check if they've
changed. It must re-read the full homepage or skill doc every time. With an ETag tied to
the build number, the agent can send `If-None-Match` and get a `304 Not Modified` if
nothing changed since its last fetch — confirming it's operating on the latest version.

## Proposed Solution

### Scope

| Endpoint | Change |
|---|---|
| `GET /` (homepage) | Replace `no-store` with `ETag` based on build number. Keep `no-cache` so clients revalidate. |
| `GET /v1/docs/agent-skill` | New endpoint. Serves `docs/agent-api-skill.md` as `text/markdown` with same build-based ETag. |

The build number changes on every deploy, so the ETag naturally invalidates when the
service is updated. Between deploys the content is static — perfect for conditional requests.

### ETag Generation

```go
// Computed once at handler construction time
etag := fmt.Sprintf(`"%s"`, h.buildNumber)
```

Both endpoints share the same ETag since both change only when a new build is deployed.

### Conditional Request Handling

```go
if r.Header.Get("If-None-Match") == etag {
    w.Header().Set("ETag", etag)
    w.WriteHeader(http.StatusNotModified)
    return
}
```

### Agent Usage

An agent that has loaded the instructions can later check:

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -H 'If-None-Match: "abc123"' \
  "$BASE/v1/docs/agent-skill"
# 304 → instructions haven't changed, agent's cached copy is current
# 200 → new version available, re-read the response body
```

## Acceptance Criteria

- [ ] `GET /` returns `ETag` header based on build number
- [ ] `GET /` returns `304` when `If-None-Match` matches
- [ ] `GET /` sets `Cache-Control: no-cache` (revalidate on each request) instead of `no-store`
- [ ] `GET /v1/docs/agent-skill` exists and serves the agent skill markdown
- [ ] `GET /v1/docs/agent-skill` returns `ETag` and `Content-Type: text/markdown`
- [ ] `GET /v1/docs/agent-skill` returns `304` when `If-None-Match` matches
- [ ] Tests for both endpoints: 200 with ETag, 304 on match, 200 on stale ETag

## Implementation

### Phase 1: Add ETag to homepage

File: `internal/adapters/httpapi/handler.go`

- [ ] Change homepage `Cache-Control` from `no-store, max-age=0` to `no-cache`
- [ ] Remove `Pragma` and `Expires` headers (obsolete when using `Cache-Control`)
- [ ] Add `ETag` header from `h.buildNumber`
- [ ] Check `If-None-Match` — return 304 if matched
- [ ] Remove `no-cache` meta tags from HTML template (they conflict with server headers)

### Phase 2: Serve agent skill document

File: `internal/adapters/httpapi/handler.go`

- [ ] Embed `docs/agent-api-skill.md` using `//go:embed` (or read at startup)
- [ ] Register `GET /v1/docs/agent-skill` route
- [ ] Handler: check `If-None-Match`, return 304 or serve markdown with ETag + `Cache-Control: no-cache`

### Phase 3: Tests

File: `internal/adapters/httpapi/handler_test.go`

- [ ] Test homepage returns ETag header
- [ ] Test homepage returns 304 on matching If-None-Match
- [ ] Test homepage returns 200 on stale If-None-Match
- [ ] Test agent skill endpoint returns markdown with ETag
- [ ] Test agent skill endpoint returns 304 on matching If-None-Match

## Sources & References

### Internal References

- Homepage handler: `internal/adapters/httpapi/handler.go:113-125`
- Build number config: `internal/adapters/httpapi/handler.go:29` (Config.BuildNumber)
- Agent skill doc: `docs/agent-api-skill.md`
- Route registration: `internal/adapters/httpapi/handler.go:83-111`
