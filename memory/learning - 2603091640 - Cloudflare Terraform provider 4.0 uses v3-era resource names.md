---
tldr: Cloudflare provider ~> 4.0 resolves to early v4 which still uses cloudflare_record and cloudflare_zones (not v5 names)
category: infra
---

# Learning: Cloudflare Terraform provider ~> 4.0 uses v3-era resource names

The constraint `~> 4.0` resolves to early v4 releases which retain v3 resource names:
- `cloudflare_zones` (plural, with `filter {}` block) — NOT `cloudflare_zone` (singular, with `filter = {}` map)
- `cloudflare_record` — NOT `cloudflare_dns_record`
- `.zones[0].id` — NOT `.id`

The Context7 docs returned v5 syntax which broke our build.
Always check which minor version the constraint resolves to before changing resource names.
