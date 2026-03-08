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

- [`flake.nix`](/Users/oldroot/Repos/mailservice/flake.nix)
- [`nix/modules/mailservice-gitops.nix`](/Users/oldroot/Repos/mailservice/nix/modules/mailservice-gitops.nix)
- [`nix/hosts/truevipaccess/configuration.nix`](/Users/oldroot/Repos/mailservice/nix/hosts/truevipaccess/configuration.nix)

Runtime model:

- NixOS host
- Docker backend managed declaratively through `virtualisation.oci-containers`
- pinned API image ref in Git
- pinned mailreceive image ref in Git
- Cloudflare Tunnel container managed by NixOS
- secrets kept out of Git in `/var/lib/secrets/mailservice.env`

## What Lives In Git

Git-managed:

- host configuration
- OCI image references
- firewall/runtime shape
- Cloudflare tunnel runtime definition
- service topology

Not in Git:

- `POLAR_TOKEN`
- `POLAR_WEBHOOK_SECRET`
- `CLOUDFLARE_TUNNEL_TOKEN`
- any other secret env values

## Image Pinning

Rollouts happen by changing the pinned image refs in
[`nix/hosts/truevipaccess/configuration.nix`](/Users/oldroot/Repos/mailservice/nix/hosts/truevipaccess/configuration.nix).

Example:

```nix
services.mailserviceGitOps = {
  enable = true;
  apiImage = "ghcr.io/atvirokodosprendimai/mailservice-api:sha-abc1234";
  mailreceiveImage = "ghcr.io/atvirokodosprendimai/mailservice-mailreceive:sha-abc1234";
};
```

That removes the mutable-`latest` race from deployment.

## Secrets File

The host expects a runtime env file at:

```text
/var/lib/secrets/mailservice.env
```

This file should contain values such as:

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
CLOUDFLARE_TUNNEL_TOKEN=...
```

## Rollout

1. Build and publish commit-pinned images.
2. Update the pinned image refs in the NixOS host config.
3. Commit that change to Git.
4. Apply the new revision with `nixos-rebuild switch`.

Example:

```bash
sudo nixos-rebuild switch --flake .#truevipaccess
```

## Rollback

Rollback is revision-based:

1. revert the Git commit that changed the host config or image refs
2. apply the previous revision again

That is the key GitOps property this path is meant to provide.

## Migration Note

This baseline does not cut over the current Ubuntu production host by itself.
It defines the target NixOS deployment model so the migration can be done
deliberately instead of continuing to extend the imperative SSH workflow.
