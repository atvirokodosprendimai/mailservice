---
title: "Embedding files from a different directory with go:embed"
category: build-errors
date: 2026-03-16
tags: [go-embed, static-files, serve-markdown, cross-package]
---

## Problem

`go:embed` only works with files relative to the package directory — no `..` paths allowed.
When you need to serve a file (e.g. `docs/agent-api-skill.md`) from an HTTP handler in a different package (`internal/adapters/httpapi/`), you can't embed it directly.

## Root Cause

Go's `embed` package restricts paths to the embedding package's directory tree.
Attempting `//go:embed ../../docs/file.md` fails at build time.

## Solution

Create a tiny Go package in the directory that contains the file:

```go
// docs/embed.go
package docs

import _ "embed"

//go:embed agent-api-skill.md
var AgentAPISkill string
```

Import and pass the content through configuration:

```go
// cmd/app/main.go
import "github.com/example/project/docs"

handler := httpapi.NewHandler(httpapi.Config{
    AgentAPISkillDoc: docs.AgentAPISkill,
})
```

Serve it in the handler:

```go
func (h *Handler) handleAgentAPISkill(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    _, _ = io.WriteString(w, h.agentAPISkillDoc)
}
```

**Key benefit:** Single source of truth — the markdown file exists once in `docs/`, no copies to keep in sync.

## Prevention

When you need to serve static files from Go:
1. First check if the file is in or below the handler's package → embed directly.
2. If not, create an `embed.go` in the file's directory and pass the content via config.
3. Avoid copying files into handler packages — duplication drifts.
