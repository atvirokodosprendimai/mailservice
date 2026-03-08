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

Bootstrap helper:
- `docs/hetzner-object-storage-bootstrap.md`

State handling rules:
- do not commit backend credentials
- generate backend config in CI from secrets
- enable bucket versioning if supported
- use remote state for apply, not local state
- note: Hetzner Object Storage credentials are created in Hetzner Console, not from `HCLOUD_API`

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
- `CLOUDFLARE_TUNNEL_TOKEN`
- `DEPLOY_HOST`
- `DEPLOY_HOST_KEY`
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
1. `validate` job completes first
2. dedicated `plan` job runs `tofu init` with remote backend config from secrets
3. `plan` job generates `tfplan` and a human-readable `tfplan.txt`
4. plan artifact is uploaded for operator review
5. gated `apply` job can run only after the `plan` job succeeds
6. production environment approval can be enforced through GitHub environment protection
7. `deploy` job writes `production.env` from GitHub vars/secrets with secret-safe file permissions
8. workflow pins the SSH host key from `DEPLOY_HOST_KEY`
9. workflow uploads `compose.tunnel.yml.example` and `production.env` to `/opt/mailservice`
10. workflow runs `docker compose pull` and `docker compose up -d` on the host
11. workflow checks `${PUBLIC_BASE_URL}/healthz` with bounded retries

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
- production apply is gated behind a separate plan stage and uploaded plan artifact
- provider-specific payment/runtime secrets remain separate from infra secrets
- this design uses OpenTofu, not Terraform
- before pushing workflow or OpenTofu changes, use the local checklist in `docs/local-workflow-validation.md`
- current production hostname target is `truevipaccess.com`; see `docs/truevipaccess-deploy.md`
- current temporary ingress path is Cloudflare Tunnel; see `docs/cloudflare-tunnel-deploy.md`
- for the tunnel path, pass `CLOUDFLARE_TUNNEL_TOKEN` into the container as `TUNNEL_TOKEN`
- host-side deploy uses `compose.tunnel.yml.example` plus a generated `production.env`
- the tunnel compose file reads runtime values from `production.env`; it is not meant to hard-code production secrets
