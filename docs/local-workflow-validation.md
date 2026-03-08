# Local Workflow Validation

Use this before pushing workflow or OpenTofu changes.

## Goal

Catch simple CI failures locally:

- OpenTofu formatting drift
- OpenTofu validation errors
- missing workflow assumptions
- basic GitHub Actions job wiring issues

This is not a full replacement for GitHub-hosted runners. It is a fast pre-push filter.

## Minimum Pre-Push Checklist

For changes touching:

- `.github/workflows/hetzner-opentofu.yml`
- `infra/opentofu/**`
- `docs/hetzner-cicd.md`

run:

```bash
tofu fmt -check infra/opentofu
tofu init -backend=false infra/opentofu
tofu validate infra/opentofu
go test ./...
```

If `tofu` is not installed locally, do not skip validation silently. Either:

- install OpenTofu first, or
- expect the workflow to be the first validator and treat that as higher risk

## Recommended Local `act` Check

If `act` is installed, use it to exercise the workflow shape:

```bash
act pull_request -W .github/workflows/hetzner-opentofu.yml
```

This is useful for:

- job dependency wiring
- shell step execution
- required env and secret shape
- catching obvious YAML or action invocation mistakes

## `act` Limits

`act` is helpful, but not complete. It is less trustworthy for:

- GitHub environment protection and approvals
- artifact behavior differences
- cloud auth edge cases
- hosted-runner specific behavior

So the right order is:

1. run the direct local tool commands
2. optionally run `act`
3. rely on GitHub Actions for final confirmation

## Practical Rule

For workflow changes, always run the underlying tool commands locally first.

Example:

- if the workflow runs `tofu fmt -check`, run `tofu fmt -check` locally before push
- if the workflow runs `go test ./...`, run that locally too

This catches the cheapest failures earliest.
