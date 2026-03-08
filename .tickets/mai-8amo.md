---
id: mai-8amo
status: closed
deps: []
links: []
created: 2026-03-08T12:30:11Z
type: task
priority: 2
assignee: ~.~
tags: [deploy, cloudflare, tunnel, reverse-proxy, temporary]
---
# Use Cloudflare Tunnel as temporary reverse proxy for truevipaccess.com

Set up a temporary production ingress path using Cloudflare Tunnel in front of the Hetzner-hosted service for truevipaccess.com. Cover runtime configuration, tunnel credentials, and request routing to the API while keeping the longer-term edge/proxy setup open.

## Acceptance Criteria

Deployment docs describe how truevipaccess.com is routed through Cloudflare Tunnel
Required tunnel credentials and runtime configuration are identified
PUBLIC_BASE_URL and payment return URLs remain correct for truevipaccess.com
The temporary nature of the tunnel-based ingress is explicit so it can be replaced later

