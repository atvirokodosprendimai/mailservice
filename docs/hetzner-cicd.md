# Hetzner CI/CD

## Goal

Deploy mailservice to Hetzner using GitHub Actions and OpenTofu.

## Target Shape

Initial target:
- one Hetzner Cloud server
- firewall allowing SSH, HTTP, HTTPS, SMTP receive, and IMAP
- NixOS-native app deployment on the host

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
- `NIX_CACHE_S3_ACCESS_KEY_ID`
- `NIX_CACHE_S3_SECRET_ACCESS_KEY`
- `CLOUDFLARE_TUNNEL_TOKEN`
- `DEPLOY_HOST`
- `DEPLOY_HOST_KEY`
- `DEPLOY_USER`
- `DEPLOY_SSH_PRIVATE_KEY`
- `POLAR_WEBHOOK_SECRET`

Deployment/cache vars:
- `NIX_CACHE_S3_BUCKET`
- `NIX_CACHE_S3_ENDPOINT` (for Hetzner: `fsn1.your-objectstorage.com`, `nbg1.your-objectstorage.com`, or `hel1.your-objectstorage.com`)
- `NIX_CACHE_S3_REGION` (for Hetzner use an AWS-style value such as `eu-central-1`)

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
7. OpenTofu stops after infrastructure apply
8. production NixOS hosts are updated through the dedicated NixOS deploy workflow

### Automatic app deploy on `main`

Run:
1. `Deploy Production App` runs on `push` to `main`
2. CI builds `.#nixosConfigurations.truevipaccess.config.system.build.toplevel`
3. if S3 cache vars and credentials are configured, CI pushes the built closure to Hetzner Object Storage
4. the workflow syncs the repo contents to the NixOS host over SSH
5. the workflow runs `nixos-rebuild switch --flake .#truevipaccess` on the host
6. if S3 cache vars are configured, the host prefers the S3 cache via Nix `s3://` substituter instead of rebuilding locally
7. deploy checks the host-local API health endpoint

This is the normal release path for application changes.
Use the OpenTofu workflow when infrastructure changes are needed.

For a NixOS migration host:
1. run the same workflow with:
   - `image=<nixos snapshot or image id>`
   - `bootstrap_mode=none`
2. OpenTofu will create the server, firewall, and SSH key only
3. complete the host configuration via the NixOps migration path or the main NixOS deploy workflow

## Rollout

Recommended rollout:
1. commit the NixOS host and application changes
2. CI builds the system closure and pushes it to Hetzner S3 cache
3. apply infrastructure changes if needed
4. sync the repo to the host
5. run `nixos-rebuild switch --flake .#truevipaccess`
6. run health check

## Rollback

Rollback expectations:
- keep previous Git revisions available
- deploy by explicit Git revision, not mutable runtime state
- if infra apply fails, do not run app deploy
- if app health check fails, revert to the previous known-good revision

## Notes

- current repo includes the OpenTofu scaffold and GitHub Actions workflow
- production apply is gated behind a separate plan stage and uploaded plan artifact
- provider-specific payment/runtime secrets remain separate from infra secrets
- when `NIX_CACHE_S3_BUCKET`, `NIX_CACHE_S3_ENDPOINT`, `NIX_CACHE_S3_REGION`, `NIX_CACHE_S3_ACCESS_KEY_ID`, and `NIX_CACHE_S3_SECRET_ACCESS_KEY` are configured, production deploys use a CI-built S3 binary cache instead of host-local builds
- this design uses OpenTofu, not Terraform
- before pushing workflow or OpenTofu changes, use the local checklist in `docs/local-workflow-validation.md`
- current production hostname target is `truevipaccess.com`; see `docs/truevipaccess-deploy.md`
- current temporary ingress path is Cloudflare Tunnel; see `docs/cloudflare-tunnel-deploy.md`
- for the tunnel path, keep `TUNNEL_TOKEN` in `/var/lib/secrets/cloudflared.env`
- host-side deploy is now NixOS-native; it no longer depends on Docker Compose or GHCR mail images
- for a NixOS/custom image host, set `bootstrap_mode=none` so the workflow provisions the VM without assuming Ubuntu packages or Docker bootstrap
