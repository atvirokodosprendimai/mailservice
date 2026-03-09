# NixOS GitOps on Hetzner

This is the post-launch target deployment model for the single Hetzner VM.

## Goal

Replace the current Ubuntu plus imperative SSH deploy path with a declarative
NixOS host configuration stored in Git.

The reason is simple:

- host package drift caused real deploy failures
- mutable shell assumptions made rollouts fragile
- Git should describe both the host runtime and the pinned application artifact

## Shape

Source of truth:

- [`flake.nix`](../flake.nix)
- [`nix/modules/mailservice-gitops.nix`](../nix/modules/mailservice-gitops.nix)
- [`nix/hosts/truevipaccess/configuration.nix`](../nix/hosts/truevipaccess/configuration.nix)

Runtime model:

- NixOS host
- native systemd service for the API
- pinned mailreceive image ref in Git
- native systemd service for Cloudflare Tunnel; configure the tunnel origin to `http://127.0.0.1:8080`, not a Docker-internal hostname such as `http://api:8080`
- Docker backend still used for mailreceive until that stack is also moved to native NixOS services
- secrets kept out of Git in `/var/lib/secrets/mailservice.env`

## What Lives In Git

Git-managed:

- host configuration
- API package build and service definition
- mailreceive image reference
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
  mailreceiveImage = "ghcr.io/atvirokodosprendimai/mailservice-mailreceive:sha-abc1234";
};
```

That removes the API’s runtime dependence on GHCR and Docker.

## Mailreceive Pinning

Rollouts for the inbound mail stack still happen by changing the pinned
`mailreceiveImage` ref in
[`nix/hosts/truevipaccess/configuration.nix`](../nix/hosts/truevipaccess/configuration.nix).

## Secrets File

The host expects runtime env files at:

```text
/var/lib/secrets/mailservice.env
/var/lib/secrets/cloudflared.env
```

`/var/lib/secrets/mailservice.env` should contain values such as:

```env
PUBLIC_BASE_URL=https://truevipaccess.com
MAIL_DOMAIN=truevipaccess.com
IMAP_HOST=truevipaccess.com
IMAP_PORT=143
MAX_CONCURRENT_REQUESTS=100
POLAR_TOKEN=...
POLAR_PRICE_ID=...
POLAR_SERVER_URL=https://api.polar.sh
POLAR_SUCCESS_URL=https://truevipaccess.com/v1/payments/polar/success?checkout_id={CHECKOUT_ID}
POLAR_RETURN_URL=https://truevipaccess.com
POLAR_WEBHOOK_SECRET=...
```

`/var/lib/secrets/cloudflared.env` should contain:

```env
TUNNEL_TOKEN=...
```

## Rollout

Preferred future path: use NixOps to apply the host revision.

1. Build and publish commit-pinned images.
2. Update the pinned `mailreceiveImage` ref in the NixOS host config when needed.
3. Commit that change to Git.
4. Apply the new revision with NixOps.

Example:

```bash
nix run .#nixops-deploy
```

Manual `nixos-rebuild switch --flake .#truevipaccess` remains useful for local
debugging on the host, but it is not the preferred multi-step rollout path.

## Rollback

Rollback is revision-based:

1. revert the Git commit that changed the host config or image refs
2. apply the previous revision again

That is the key GitOps property this path is meant to provide.

## Migration Note

This baseline does not cut over the current Ubuntu production host by itself.
Use the dedicated [NixOps migration plan](nixops-migration-plan.md) for the
actual cutover sequence.
