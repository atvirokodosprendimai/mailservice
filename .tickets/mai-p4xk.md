---
id: mai-p4xk
status: closed
deps: []
links: []
created: 2026-03-08T18:24:30Z
type: task
priority: 2
assignee: ~.~
tags: [agents, website, instructions, edproof, onboarding, product]
---
# Create published agent instructions for autonomous mailbox claim flow

Create a website-published instruction block for agents that tells them exactly how to obtain a mailbox autonomously without leaking internal context or stopping for unnecessary operator questions.

## Acceptance Criteria

The published instructions define the default action when no EdProof key exists: generate one and continue
The instructions explain same key => same mailbox and different key => different mailbox
The instructions describe the exact claim, pay, and resolve flow against this service
The instructions avoid internal references such as Moltbook or agent-specific private context
The instructions tell the agent when to ask the operator and when not to ask
The website presentation is concise and instruction-first, with a copy/pasteable block similar in spirit to Moltbook's agent onboarding panel
