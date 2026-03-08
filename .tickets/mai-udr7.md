---
id: mai-udr7
status: closed
deps: []
links: []
created: 2026-03-08T12:13:36Z
type: task
priority: 2
assignee: ~.~
tags: [deploy, dns, tls, hetzner, production]
---
# Deploy mailservice under truevipaccess.com

Configure infrastructure and runtime so the service is reachable under truevipaccess.com in production. Cover DNS, PUBLIC_BASE_URL, TLS termination, and any mail/HTTP host-specific configuration needed for the current Hetzner deployment path.

## Acceptance Criteria

Deployment plan identifies how truevipaccess.com points to the production host
Runtime configuration uses truevipaccess.com as the public base URL where appropriate
TLS termination or certificate provisioning for truevipaccess.com is defined
Any mail or HTTP hostname assumptions impacted by the domain are documented before rollout

