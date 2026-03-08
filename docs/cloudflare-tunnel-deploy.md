# Cloudflare Tunnel Deployment

Temporary ingress plan for `truevipaccess.com`.

## Goal

Expose the API at:

- `https://truevipaccess.com`

without managing a public reverse proxy on the Hetzner host yet.

This is a temporary ingress path. It can be replaced later by a direct edge proxy or load balancer.

## Shape

Use `cloudflared` alongside the app runtime on the host:

- `api` listens on the internal Docker network
- `cloudflared` publishes the tunnel to Cloudflare
- Cloudflare routes `truevipaccess.com` through the tunnel to `http://api:8080`

## Required Secrets

- `CLOUDFLARE_TUNNEL_TOKEN`

Recommended repo/runtime secret handling:

- store the token in GitHub Actions secrets
- write it into the production env/runtime file on deploy
- do not commit it into OpenTofu variables or repo files
- keep the generated runtime file readable only by the deploy user

## Runtime Values

Production API values should still be:

```env
PUBLIC_BASE_URL=https://truevipaccess.com
POLAR_SUCCESS_URL=https://truevipaccess.com/v1/payments/polar/success?checkout_id={CHECKOUT_ID}
POLAR_RETURN_URL=https://truevipaccess.com
POLAR_WEBHOOK_SECRET=<secret>
```

Tunnel-specific runtime value:

```env
CLOUDFLARE_TUNNEL_TOKEN=<secret>
```

At runtime, map it into the container as:

```env
TUNNEL_TOKEN=${CLOUDFLARE_TUNNEL_TOKEN}
```

## Docker Compose Shape

Add a `cloudflared` service that runs:

```text
cloudflared tunnel --no-autoupdate run
```

with:

- image `cloudflare/cloudflared:2026.2.0`
- `TUNNEL_TOKEN` from environment
- compose maps `CLOUDFLARE_TUNNEL_TOKEN` into `TUNNEL_TOKEN`
- dependency on the API container
- no published API host port in the tunnel-based production shape

Use:

- `compose.yml.example` for local development
- `compose.tunnel.yml.example` for the tunnel-based production shape
- `deploy/production.env.example` as the checked-in template for the host runtime file

## Cloudflare Routing

In Cloudflare:

1. create a tunnel
2. configure public hostname `truevipaccess.com`
3. point it to `http://api:8080` inside the Docker network
4. issue the tunnel token for runtime use

No direct `A` record to Hetzner is required for this temporary path.

## Notes

- This is an ingress choice only. It does not change the mailbox domain.
- The tunnel path is meant to get production access working quickly.
- Longer-term direct TLS/reverse-proxy setup can replace it later.
