# Hetzner CI/CD

## Goal

Deploy mailservice to Hetzner using GitHub Actions and OpenTofu.

## Target Shape

Initial target:
- one Hetzner Cloud server
- firewall allowing SSH, HTTP, HTTPS, SMTP receive, and IMAP
- Docker-based app deployment on the host

This is intentionally small. Scale-out can come later.

## OpenTofu State

Recommended state backend:
- S3-compatible remote state
- Hetzner Object Storage is a practical option

State handling rules:
- do not commit backend credentials
- generate backend config in CI from secrets
- enable bucket versioning if supported
- use remote state for apply, not local state

OpenTofu backend reference:
- OpenTofu `s3` backend docs: https://opentofu.org/docs/v1.9/language/settings/backends/s3/

## Required GitHub Secrets

Infrastructure:
- `HCLOUD_API`
- `HETZNER_SSH_PUBLIC_KEY`
- `TOFU_STATE_BUCKET`
- `TOFU_STATE_REGION`
- `TOFU_STATE_ENDPOINT`
- `TOFU_STATE_ACCESS_KEY`
- `TOFU_STATE_SECRET_KEY`

Deployment/runtime:
- `APP_ENV_PRODUCTION`
- `DEPLOY_HOST`
- `DEPLOY_USER`
- `DEPLOY_SSH_PRIVATE_KEY`

## Workflow Shape

### Pull requests

Run:
- `tofu fmt -check`
- `tofu init -backend=false`
- `tofu validate`
- `tofu plan` without apply

### Manual production deploy

Run:
1. `tofu init` with remote backend config from secrets
2. `tofu plan`
3. optional `tofu apply`
4. upload runtime env file to host
5. deploy Docker Compose stack on target host

## Rollout

Recommended rollout:
1. build and publish image
2. apply infrastructure changes if needed
3. upload env/runtime config
4. pull new image on host
5. restart service with Docker Compose
6. run health check

## Rollback

Rollback expectations:
- keep previous image tag available
- deploy by explicit image tag, not mutable assumptions alone
- allow manual workflow input for rollback tag
- if infra apply fails, do not run app deploy
- if app health check fails, revert to previous known-good image tag

## Notes

- current repo includes the OpenTofu scaffold and GitHub Actions workflow
- provider-specific payment/runtime secrets remain separate from infra secrets
- this design uses OpenTofu, not Terraform
