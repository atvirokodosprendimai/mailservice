---
title: "feat: Public launch — agent mailbox service"
type: feat
status: completed
date: 2026-03-16
---

# feat: Public launch — agent mailbox service

## Overview

Launch the mailbox service publicly as **private email infrastructure for LLM agents**.
Core value prop: each agent gets its own stable email address, bound to an Ed25519 key.
The service works. This plan covers positioning, packaging, and go-to-market — not new features.

## Problem Statement / Motivation

The service is fully functional — challenge-response auth, Polar payments, gift coupons, proactive expiry, smoke tests — but it has no public presence beyond the landing page.
LLM agents (like OpenClaw) need private mailboxes to receive confirmations, sign up for services, and get notifications.
There's no competing "mailbox-as-a-service for agents" product.
First-mover advantage matters.

## Proposed Solution

Ship a focused public launch in 4 phases: fix gaps, polish presence, seed first users, then announce.

## Pre-Launch Fixes (must-do before any announcement)

### 1. Serve agent-api-skill.md via HTTP

The landing page references `GET /docs/agent-api-skill.md` but no route exists — agents get a 404.

- **File:** `internal/adapters/httpapi/handler.go`
- **Fix:** Add route `GET /docs/agent-api-skill.md` that serves the file content with `text/markdown` content type
- **Alternative:** Embed the file content as a Go constant (like the homepage) to avoid filesystem dependency

### 2. Gate mock payment endpoint

`GET /mock/pay/{sessionID}` is registered unconditionally in production.

- **File:** `internal/adapters/httpapi/handler.go:106`
- **Fix:** Only register when payment gateway is the mock gateway (dev mode)

### 3. Price consistency

`MAILBOX_PRICE_CENTS` defaults to 299 but all copy says 1 EUR/month.

- **File:** `internal/platform/config/config.go:139`
- **Fix:** Update default to 100 (1 EUR in cents) or document that 299 is intentional and update copy

### 4. Add OpenGraph meta tags to landing page

For social sharing when links are posted on HN/Reddit/Twitter.

- **Fix:** Add `og:title`, `og:description`, `og:type`, `og:url` meta tags to `homePageHTMLTemplate`
- Keep it minimal — no image needed initially

### 5. Add footer to landing page

Currently no contact info, no link to repo, no legal.

- **Fix:** Add a minimal footer with: GitHub repo link, contact email, and a one-line "AGPL v3.0" notice
- Terms of service and privacy policy can be deferred — for a 1 EUR/month developer tool, a contact email is sufficient at launch

## Launch Presence

### 6. Write a launch README section

The README is comprehensive but developer-focused. Add a "What is this?" section at the top that positions the product for agents:

> Private inbound email for LLM agents. Each agent gets a stable mailbox bound to its Ed25519 key. 1 EUR/month, 100 MB, IMAP + HTTP API.

### 7. Prepare HN / Reddit post

Draft a "Show HN" or "Show Reddit" post. Key angles:
- "I built a mailbox service for LLM agents"
- Problem: agents need email addresses to sign up for services, receive confirmations
- Solution: Ed25519 key → stable mailbox identity, challenge-response auth, 1 EUR/month
- Differentiator: no free tier abuse, no SMTP (inbound only), clean billing via Polar
- Open source (AGPL v3.0)

### 8. OpenClaw integration demo

Create a concrete demo showing an OpenClaw agent:
1. Generating a key
2. Claiming a mailbox with OPENCLAWS coupon
3. Signing up for a service using the mailbox email
4. Reading the confirmation email via API

This becomes the launch artifact — a video, gif, or blog post showing the end-to-end flow.

## Go-To-Market Strategy

### Channels (prioritized)

1. **OpenClaw community** — direct outreach, OPENCLAWS coupon (23 free slots)
2. **Hacker News "Show HN"** — developer audience, open source angle
3. **r/selfhosted, r/LocalLLaMA** — privacy + agent audience overlap
4. **Twitter/X** — AI agent builder community
5. **Product Hunt** — defer unless HN generates traction

### Positioning

**One-liner:** "Private email for LLM agents — stable mailbox identity, bound to a key."

**For whom:** Agent builders who need their agents to have email addresses.

**Against whom:** Temp mail services (too ephemeral), Gmail API (too complex, not agent-native), self-hosted Postfix (too much ops).

**Why now:** Agents are getting more autonomous. They need communication infrastructure that doesn't require human intervention to set up.

## Acceptance Criteria

- [x] `GET /docs/agent-api-skill.md` returns the skill document (not 404)
- [x] Mock payment endpoint only registered in dev mode
- [x] Price default matches copy (1 EUR = 100 cents)
- [x] Landing page has OpenGraph meta tags
- [x] Landing page has a footer with GitHub link and contact
- [x] README has a clear "What is this?" opening for the agent audience
- [x] Show HN / Reddit draft written
- [x] OpenClaw integration demo documented or recorded

## Success Metrics

- 10+ mailboxes claimed in first week after announcement
- OPENCLAWS coupon usage > 5 (of 23 available)
- Positive HN/Reddit reception (>50 upvotes on Show HN)
- At least 1 paid (non-coupon) user in first month

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| HN/Reddit audience doesn't understand "agent email" | Lead with a concrete demo, not abstract positioning |
| Abuse (spam signups) | Inbound-only, no SMTP. Ed25519 key requirement + payment is natural rate limiting |
| Scale concerns (SQLite) | Current architecture handles hundreds of mailboxes fine. Scale is a good problem to have |
| Domain name (truevipaccess.com) doesn't communicate agent email | Consider whether a domain rename is worth the effort. Probably not for launch — the product speaks for itself |

## Sources

- Landing page: `internal/adapters/httpapi/handler.go:147-408`
- Agent API skill: `docs/agent-api-skill.md`
- Website copy: `docs/website-copy.md`
- Gift coupons plan: `docs/plans/2026-03-12-001-feat-gift-coupon-codes-plan.md`
- Use cases: `docs/use-cases.md`
- Story/positioning: `STORY.md`
