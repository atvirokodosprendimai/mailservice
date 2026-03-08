# Architecture Docs

This directory is the repo-local architecture baseline for `mailservice`.

It adopts the C4 way of thinking:
- system context
- container view
- deployment view

It does not yet commit the repo to a generated-diagram toolchain.

## Why

The project needs long-lived architecture documentation that:
- stays in Git next to the code
- is easy to review in pull requests
- matches the current hexagonal Go codebase
- can later be exported into a richer modelling tool if needed

## Current Files

- `tooling-decision.md`
- `system-context.md`
- `container-view.md`
- `deployment-view.md`

## Maintenance Rules

- Update these docs when adding or removing major adapters, external systems, or deployment steps.
- Prefer concrete names from the codebase over vague boxes and arrows.
- Keep business and billing language aligned with the product docs:
  inbound mailbox access only, no SMTP submission.
- If the repo later adopts full C4InterFlow modelling, keep these files as the human-readable overview and generate diagrams from a single checked-in architecture model.
