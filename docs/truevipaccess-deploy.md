# truevipaccess.com Deployment

This is the production hostname plan for mailservice.

## Goal

Serve the API under:

- `https://truevipaccess.com`

Use this hostname as the public base URL for user-facing links and payment return flow.

## Current Ingress

Current temporary ingress choice:

- Cloudflare Tunnel in front of the Hetzner-hosted runtime

See:

- `docs/cloudflare-tunnel-deploy.md`

## DNS

For the tunnel-based temporary path, Cloudflare manages hostname routing and a direct public `A` record to Hetzner is not required.

If you switch later to direct host exposure, then:

- apex `A` record:
  - `truevipaccess.com -> <Hetzner server IPv4>`
- apex `AAAA` record:
  - optional unless IPv6 is configured on the deployed host and TLS proxy

The OpenTofu output `server_ipv4` remains the fallback direct-host target.

## Runtime Configuration

Production runtime should use:

```env
PUBLIC_BASE_URL=https://truevipaccess.com
POLAR_SUCCESS_URL=https://truevipaccess.com/v1/payments/polar/success?checkout_id={CHECKOUT_ID}
POLAR_RETURN_URL=https://truevipaccess.com
POLAR_WEBHOOK_SECRET=<secret>
```

If this hostname is also used as the mailbox domain later, that should be a separate decision. The current HTTP deployment target does not require changing `MAIL_DOMAIN`.

## TLS

For the current temporary path, TLS terminates at Cloudflare.

Later direct-host option:

Terminate TLS on the production host with a reverse proxy.

Recommended minimal shape:

- Caddy or Nginx on ports `80` and `443`
- automatic Let's Encrypt certificate for `truevipaccess.com`
- reverse proxy traffic to the API container on an internal port

Minimum TLS rollout steps:

1. point DNS `A` record to the Hetzner server
2. bring up the reverse proxy with `truevipaccess.com`
3. let the proxy obtain the certificate
4. verify `https://truevipaccess.com/healthz`

## Hetzner Notes

The current firewall already allows:

- `80/tcp`
- `443/tcp`

So no firewall change is required if you later switch to direct host exposure. The tunnel path does not depend on public inbound `80/443`.

## Validation Checklist

Before calling the deployment complete:

1. `curl -I https://truevipaccess.com/healthz` returns `200`
2. payment return URLs use `https://truevipaccess.com`
3. Cloudflare Tunnel points to the API runtime
4. no user-facing links still point at localhost or a temporary host
