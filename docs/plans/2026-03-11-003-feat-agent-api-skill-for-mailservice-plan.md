---
title: "feat: Agent API skill for mailservice"
type: feat
status: active
date: 2026-03-11
---

# Agent API Skill for Mailservice

## Overview

Create a standalone, loadable skill document that any AI agent can use to autonomously interact with the TrueVIP Access Mailbox API — from key generation through mailbox claim, payment, IMAP access resolution, and message reading.

## Problem Statement

The current agent guidance is scattered across three locations:
1. **Homepage prompt** (`handler.go:404-414`) — 10 lines, covers only claim/resolve basics
2. **README.md** — human-oriented, mixes setup instructions with API docs
3. **AGENTS.md** — focused on contributing to the codebase, not using the API

An AI agent arriving at the service has no single document that answers: "How do I get a mailbox and read email?" The agent must piece together the flow from HTML, README curl examples, and trial-and-error. Common failure modes:
- Not understanding the 3-step challenge-response flow
- Sending signatures with newlines (base64 from ssh-keygen is multi-line)
- Not knowing that challenges expire in 30 seconds
- Not knowing how to handle payment (the claim returns a `payment_url`)
- Trying to resolve access before paying
- Not stripping the signature armor from `ssh-keygen -Y sign` output

## Proposed Solution

A single markdown skill file at `docs/agent-api-skill.md` that an agent can load (via MCP, system prompt injection, or manual read) to operate the full API autonomously. The skill follows a decision-tree structure:

### Skill Structure

```
1. Prerequisites (key generation)
2. Decision: Do I have an existing key?
   → Yes: skip to step 3
   → No: generate one
3. Claim a mailbox (challenge → sign → claim)
4. Handle payment (follow payment_url)
5. Resolve access (challenge → sign → resolve)
6. Read messages (IMAP via API)
7. Error reference (common errors → fixes)
```

### Key Design Decisions

- **Self-contained** — no wiki links or cross-references that require loading other files
- **Decision-tree, not narrative** — agents need branching logic, not prose
- **Exact commands** — every step includes the exact curl/ssh-keygen command
- **Error recovery** — each error response maps to a concrete next action
- **Idempotent guidance** — "if you already have X, skip to step Y"

## Acceptance Criteria

- [ ] `docs/agent-api-skill.md` exists and covers the full lifecycle
- [ ] Key generation: `ssh-keygen -t ed25519` with correct flags
- [ ] Challenge-response: exact 3-step flow with all JSON fields
- [ ] Signature handling: newline stripping, base64 armor removal, both raw and SSHSIG formats
- [ ] Payment: what `payment_url` means, how to wait for activation
- [ ] Access resolution: IMAP credentials extraction
- [ ] Message reading: list and get endpoints with access tokens
- [ ] Error reference: every error message the API returns → what to do
- [ ] 30-second challenge TTL documented with retry guidance
- [ ] Tested: an agent can follow the skill end-to-end against a running instance
- [ ] Homepage prompt (`handler.go`) updated to reference the skill or include key improvements

## Implementation Phases

### Phase 1: Write the skill document

File: `docs/agent-api-skill.md`

- [ ] Prerequisites section (key generation, key reuse rules)
- [ ] Full claim flow with exact JSON payloads and curl commands
- [ ] Signature creation section (both `ssh-keygen -Y sign` and raw Ed25519)
- [ ] Payment handling (what to do with `payment_url`, polling for status)
- [ ] Access resolution flow
- [ ] Message reading (list messages, get message by UID)
- [ ] Error reference table (HTTP status + error message → action)
- [ ] Decision tree for "do I need a new key or reuse existing?"

### Phase 2: Update homepage prompt

File: `internal/adapters/httpapi/handler.go`

- [ ] Improve the embedded agent prompt (lines 404-414) with the most critical guidance
- [ ] Add signature newline warning
- [ ] Reference the skill document URL if the service has a docs endpoint

### Phase 3: Validate

- [ ] Walk through the skill as an agent against a local instance
- [ ] Verify every curl command works
- [ ] Verify error messages match what the API actually returns

## Sources & References

### Internal References

- Homepage agent prompt: `internal/adapters/httpapi/handler.go:404-414`
- Challenge-response logic: `internal/adapters/identity/edproof/challenge.go`
- Handler endpoints: `internal/adapters/httpapi/handler.go:83-111`
- Request/response types: `internal/adapters/httpapi/handler.go` (various structs)
- Key-bound mailbox spec: `docs/key-bound-mailbox-spec.md`
- SSHSIG solution doc: `docs/solutions/security-issues/ed25519-challenge-response-auth.md`
