---
id: mai-3aev
status: closed
deps: []
links: []
created: 2026-03-08T18:19:30Z
type: task
priority: 1
assignee: ~.~
tags: [deploy, cicd, ghcr, immutable, launch]
---
# Deploy immutable image tags instead of latest

Remove the race between image publishing and production deployment by deploying commit-specific image tags and waiting for those tags to exist before the host pulls them.

## Acceptance Criteria

Production deploy no longer references mutable `latest` image tags
Deploy workflow uses commit-specific image tags for both API and mailreceive images
Deploy workflow waits for the required GHCR tags to be available before SSH deployment
Production runtime files receive the image references explicitly
Docs explain the immutable-tag rollout model
