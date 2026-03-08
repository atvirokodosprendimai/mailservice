# Hetzner CI/CD

## Goal

Deploy mailservice to Hetzner using GitHub Actions and OpenTofu.

## Target Shape

Initial target:
- one Hetzner Cloud server
- firewall allowing SSH, HTTP, HTTPS, SMTP receive, and IMAP
- Docker-based app deployment on the host

Alternate migration target:
- one Hetzner Cloud server created from a prebuilt NixOS/custom image or snapshot
- `bootstrap_mode=none` so OpenTofu creates the VM but does not inject the Ubuntu Docker bootstrap
- NixOps / NixOS configuration takes over after provisioning

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
- keep an explicit `backend "s3" {}` block in `infra/opentofu/versions.tf`
- when using Hetzner Object Storage, use the Hetzner endpoint in `TOFU_STATE_ENDPOINT`, but set `TOFU_STATE_REGION` to a valid AWS-style region string such as `eu-central-1`

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
- `POLAR_WEBHOOK_SECRET`

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
8. deploy computes immutable image refs for the current commit SHA
9. workflow waits for those GHCR image tags to exist before continuing
10. workflow pins the SSH host key from `DEPLOY_HOST_KEY`
11. workflow uploads `compose.tunnel.yml.example` and `production.env` to `/opt/mailservice`
12. workflow runs compose on the host against those exact image tags
13. workflow checks the host-local API health endpoint with bounded retries

### Automatic app deploy on `main`

Run:
1. `Docker Build and Push` publishes immutable GHCR images on `push` to `main`
2. `Deploy Production App` also runs on `push` to `main`
3. deploy resolves the exact `sha-<commit>` image tags for that merge commit
4. deploy waits until those exact image manifests exist in GHCR
5. deploy uploads `compose.tunnel.yml.example` and a generated `production.env`
6. deploy rolls the host to that exact app revision
7. deploy checks the host-local API health endpoint

This is the normal release path for application changes.
Use the OpenTofu workflow when infrastructure changes are needed.

For a NixOS migration host:
1. run the same workflow with:
   - `image=<nixos snapshot or image id>`
   - `bootstrap_mode=none`
2. OpenTofu will create the server, firewall, and SSH key only
3. the Ubuntu compose deploy job is skipped
4. complete the host configuration via the NixOps migration path

## Rollout

Recommended rollout:
1. build and publish image
2. resolve exact immutable image tags for the commit being deployed
3. apply infrastructure changes if needed
4. upload env/runtime config
5. pull those exact images on host
6. restart service with Docker Compose
7. run health check

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
- production deploy uses immutable GHCR tags of the form `sha-<commit>` rather than relying on `latest`
- for a NixOS/custom image host, set `bootstrap_mode=none` so the workflow provisions the VM without assuming Ubuntu packages or Docker bootstrap
