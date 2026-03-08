---
id: mai-25e8
status: closed
deps: []
links: []
created: 2026-03-08T09:50:44Z
type: feature
priority: 1
assignee: ~.~
parent: mai-sehx
tags: [backend, edproof, architecture]
---
# Add key proof verifier port and edproof adapter

Introduce a core verification port for key proofs and add an edproof adapter that verifies submitted proofs and returns a protocol-neutral VerifiedKey structure. Keep edproof-specific details out of the core service layer.

## Acceptance Criteria

Core ports include a key proof verifier abstraction\nAdapter verifies edproof input and returns stable key fingerprint data\nCore services do not import edproof-specific types\nUnit tests cover valid and invalid proof verification paths

