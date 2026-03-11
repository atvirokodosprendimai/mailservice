# Architecture Docs

This directory is the repo-local architecture baseline for `mailservice`.

It adopts the C4 way of thinking:
- system context
- container view
- deployment view

It also documents request-level runtime behavior for the key-bound flow.

It does not yet commit the repo to a generated-diagram toolchain.

## Why

The project needs long-lived architecture documentation that:
- stays in Git next to the code
- is easy to review in pull requests
- matches the current hexagonal Go codebase
- can later be exported into a richer modelling tool if needed

## Current Files

- `overview.md`
- `tooling-decision.md`
- `system-context.md`
- `container-view.md`
- `deployment-view.md`
- `runtime-sequences.md`
- `diagrams/README.md` (diagram source scripts)
- `images/*.svg` (docs-embedded generated diagram outputs)
- `images/*.png` (generated preview/export outputs)

## Suggested Reading Order

1. `overview.md`
2. `system-context.md`
3. `container-view.md`
4. `runtime-sequences.md`
5. `deployment-view.md`
6. `tooling-decision.md`

## Maintenance Rules

- Update these docs when adding or removing major adapters, external systems, or deployment steps.
- Prefer concrete names from the codebase over vague boxes and arrows.
- Keep business and billing language aligned with the product docs:
  inbound mailbox access only, no SMTP submission.
- If the repo later adopts full C4InterFlow modelling, keep these files as the human-readable overview and generate diagrams from a single checked-in architecture model.
