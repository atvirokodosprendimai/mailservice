# NixOS GitOps on Hetzner

This is the post-launch target deployment model for the single Hetzner VM.

## Goal

Replace the current Ubuntu plus imperative SSH deploy path with a declarative
NixOS host configuration stored in Git.

The reason is simple:

- host package drift caused real deploy failures
- mutable shell assumptions made rollouts fragile
- Git should describe the host runtime and the application it runs

## Shape

Source of truth:

- [`flake.nix`](../flake.nix)
- [`nix/modules/mailservice-gitops.nix`](../nix/modules/mailservice-gitops.nix)
- [`nix/hosts/truevipaccess/configuration.nix`](../nix/hosts/truevipaccess/configuration.nix)

Runtime model:

- NixOS host
- native systemd service for the API
- native Postfix and Dovecot services for inbound mail
- native systemd service for Cloudflare Tunnel; configure the tunnel origin to `http://127.0.0.1:8080`, not a Docker-internal hostname such as `http://api:8080`
- secrets kept out of Git in `/var/lib/secrets/mailservice.env`

## What Lives In Git

Git-managed:

- host configuration
- API package build and service definition
- native mail service configuration
- firewall/runtime shape
- Cloudflare tunnel runtime definition
- service topology

Not in Git:

- `POLAR_TOKEN`
- `POLAR_WEBHOOK_SECRET`
- `CLOUDFLARE_TUNNEL_TOKEN`
- any other secret env values

## API Packaging

The API is built directly from the repo as a Nix package and run under systemd.

Example:

```nix
services.mailserviceGitOps = {
  enable = true;
  mailDomain = "truevipaccess.com";
};
```

That removes the runtime dependence on GHCR and Docker.

## Secrets and Configuration

The deploy workflow writes `/var/lib/secrets/mailservice.env` from GitHub secrets and
variables on every deploy. No manual SSH is needed to manage app secrets.

### GitHub Secrets (sensitive values)

| Secret | Purpose |
|--------|---------|
| `POLAR_TOKEN` | Polar API token |
| `POLAR_WEBHOOK_SECRET` | Polar webhook signature verification |
| `UNSEND_KEY` | Unsend transactional email API key |
| `DEPLOY_SSH_PRIVATE_KEY` | SSH key for deploy host access |
| `DEPLOY_HOST_KEY` | Known hosts entry for deploy host |
| `DEPLOY_HOST` | Deploy target hostname/IP |
| `DEPLOY_USER` | SSH user on deploy host |

### GitHub Variables (non-sensitive config)

| Variable | Example |
|----------|---------|
| `PUBLIC_BASE_URL` | `https://truevipaccess.com` |
| `MAIL_DOMAIN` | `truevipaccess.com` |
| `IMAP_HOST` | `truevipaccess.com` |
| `IMAP_PORT` | `143` |
| `MAX_CONCURRENT_REQUESTS` | `100` |
| `POLAR_PRODUCT_ID` | `01f68f36-4b6f-402a-b670-c7ebde03a836` |
| `POLAR_SERVER_URL` | `https://api.polar.sh` |
| `POLAR_SUCCESS_URL` | `https://truevipaccess.com/v1/payments/polar/success?checkout_id={CHECKOUT_ID}` |
| `POLAR_RETURN_URL` | `https://truevipaccess.com` |
| `UNSEND_BASE_URL` | `https://unsend.admin.lt/api` |
| `UNSEND_FROM_EMAIL` | `noreply@truevipaccess.com` |
| `UNSEND_FROM_NAME` | `MailService` |

The deploy validates that required vars are non-empty before applying.
`BUILD_NUMBER` and `CACHE_BUSTER` are computed by CI and injected automatically.

### Cloudflare Tunnel

`/var/lib/secrets/cloudflared.env` is not managed by the deploy workflow.
Create it manually on the host:

```env
TUNNEL_TOKEN=...
```

## Rollout

Preferred future path: use NixOps to apply the host revision.

1. Commit the host and application changes to Git.
2. CI builds `.#nixosConfigurations.truevipaccess.config.system.build.toplevel`.
3. If S3 cache vars/secrets are configured, CI pushes the closure to Hetzner Object Storage.
4. Apply the new revision with NixOps or `nixos-rebuild switch`.

Example:

```bash
nix run .#nixops-deploy
```

Manual `nixos-rebuild switch --flake .#truevipaccess` remains useful for local
debugging on the host, but it is not the preferred multi-step rollout path.

## Binary Cache

Recommended production shape:

- CI builds the NixOS closure on the runner
- CI pushes that closure to a Hetzner S3-compatible bucket
- the host switches using that cache (`s3://` substituter) instead of rebuilding locally

Required GitHub configuration:

- secret: `NIX_CACHE_S3_ACCESS_KEY_ID`
- secret: `NIX_CACHE_S3_SECRET_ACCESS_KEY`
- var: `NIX_CACHE_S3_BUCKET`
- var: `NIX_CACHE_S3_ENDPOINT`
- var: `NIX_CACHE_S3_REGION`

The deploy workflow passes the configured `s3://` substituter directly to
`nixos-rebuild`, so the host can consume the prebuilt closure without
additional persistent Nix configuration.

## Rollback

Rollback is revision-based:

1. revert the Git commit that changed the host config
2. apply the previous revision again

That is the key GitOps property this path is meant to provide.

## Migration Note

This baseline does not cut over the current Ubuntu production host by itself.
Use the dedicated [NixOps migration plan](nixops-migration-plan.md) for the
actual cutover sequence.
