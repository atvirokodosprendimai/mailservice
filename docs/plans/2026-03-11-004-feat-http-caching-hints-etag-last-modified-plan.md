---
title: "feat: ETag on homepage for agent cache validation"
type: feat
status: active
date: 2026-03-11
---

# ETag on Homepage for Agent Cache Validation

## Overview

Add an ETag (based on the build number) to `GET /` so agents can send `If-None-Match` and
get `304 Not Modified` when the instructions haven't changed. This lets an agent confirm
it's operating on the latest version without re-downloading the full page.

## Problem Statement

An agent that has previously loaded the API instructions has no way to check if they've
changed. It must re-read the full homepage every time. With an ETag tied to the build
number, the agent can revalidate with a single conditional request.

## Proposed Solution

- Replace `Cache-Control: no-store, max-age=0` with `Cache-Control: no-cache`
- Remove obsolete `Pragma` and `Expires` headers
- Add `ETag` header from `h.buildNumber`
- Check `If-None-Match` — return 304 if matched

The build number changes on every deploy, so the ETag naturally invalidates when the
service is updated. Between deploys the content is static.

## Acceptance Criteria

- [x] `GET /` returns `ETag` header based on build number
- [x] `GET /` returns `304 Not Modified` when `If-None-Match` matches
- [x] `GET /` sets `Cache-Control: no-cache` instead of `no-store`
- [x] Existing homepage test updated for new cache headers
- [x] New tests: 304 on matching ETag, 200 on stale ETag

## Sources & References

- Homepage handler: `internal/adapters/httpapi/handler.go:113-125`
- Build number: `internal/adapters/httpapi/handler.go:29` (Config.BuildNumber)
- Existing test: `internal/adapters/httpapi/handler_test.go` (TestHandleHomeReturnsLandingPage)
