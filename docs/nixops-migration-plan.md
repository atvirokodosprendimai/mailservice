# NixOps Migration Plan

## Goal

Replace the current Ubuntu production host with a fresh NixOS host managed by
NixOps, while keeping `truevipaccess.com` available throughout the cutover.

## Preconditions

- the repo branch with the NixOS GitOps baseline and NixOps files is merged
- pinned API and mailreceive image refs are updated to real `sha-...` tags
- a fresh Hetzner VM is available for the NixOS target
- the operator has SSH access to that fresh host

## Phase 1: Prepare the NixOS Host

1. Install NixOS on a fresh Hetzner VM.
2. Ensure SSH access works as `root`.
3. Create the secret directories and files:
   - `/var/lib/secrets/mailservice.env`
   - `/var/lib/secrets/cloudflared.env`
4. Populate the files with production values.

`mailservice.env` must include at least:

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

`cloudflared.env` must include:

```env
TUNNEL_TOKEN=...
```

## Phase 2: Pin Real Images in Git

1. Update `nix/hosts/truevipaccess/configuration.nix`.
2. Replace both `sha-PLACEHOLDER` values with real GHCR image tags.
3. Commit the pinned image refs.

This step is required because the module now asserts against placeholder image
values.

## Phase 3: Create NixOps Deployment State

From the repo root:

```bash
nix run .#nixops-create
```

Optional environment overrides:

- `NIXOPS_DEPLOYMENT`
- `NIXOPS_TARGET_HOST`
- `NIXOPS_TARGET_USER`
- `NIXOPS_STATE`

Default target values match the current production host shape:

- deployment name: `mailservice-truevipaccess`
- target user: `root`
- target host: `46.62.133.191`

## Phase 4: Deploy the NixOS Host

From the repo root:

```bash
nix run .#nixops-deploy
```

Validate on the host after deploy:

- API listens on `127.0.0.1:8080`
- mailreceive listens on `25` and `143`
- cloudflared is running
- `curl http://127.0.0.1:8080/healthz` succeeds on the host

## Phase 5: Cut Over Public Traffic

1. Point the Cloudflare Tunnel/DNS for `truevipaccess.com` to the new NixOS
   host.
2. Verify:
   - `https://truevipaccess.com/`
   - `https://truevipaccess.com/healthz`
   - mailbox claim flow
   - Polar payment completion
   - IMAP resolve flow

Keep the Ubuntu host intact until these checks pass.

## Phase 6: Retire the Ubuntu Path

After the NixOS host is stable:

1. stop using the imperative Ubuntu deployment path
2. remove or deprecate the old SSH/Compose deployment docs
3. migrate automation to update pinned images and run NixOps instead

## Rollback

If the NixOS host deploys but the rollout is bad:

```bash
nix run .#nixops-rollback
```

If public cutover fails before the Ubuntu host is retired:

1. point Cloudflare back to the Ubuntu host
2. keep serving traffic from Ubuntu
3. debug the NixOS host out of band
