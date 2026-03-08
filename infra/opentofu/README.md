# OpenTofu

This directory contains the initial Hetzner Cloud scaffold for mailservice.

Use it as the deployment baseline for GitHub Actions.

Files:
- `versions.tf` provider and OpenTofu requirements
- `variables.tf` input variables
- `main.tf` server, firewall, and SSH key resources
- `cloud-init.tftpl` base host bootstrap

Recommended backend:
- remote S3-compatible backend configured in CI from secrets

Important when using Hetzner Object Storage:
- keep the explicit `backend "s3" {}` block in `versions.tf`
- use the Hetzner endpoint in `TOFU_STATE_ENDPOINT`
- use an AWS-style value such as `eu-central-1` for `TOFU_STATE_REGION`

Do not commit state credentials.

Production target hostname:
- `truevipaccess.com`

Set:
- `public_hostname=truevipaccess.com`
- `public_base_url=https://truevipaccess.com`

Then use the `server_ipv4` output as the DNS `A` record target.
