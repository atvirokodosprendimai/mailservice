---
title: "Missing EDPROOF_HMAC_SECRET in smoke deploy pipeline"
category: integration-issues
date: 2026-03-11
tags: [ci-cd, deploy, config, secrets, nixos, github-actions]
components: [deploy-smoke.yml, config.go]
severity: critical
resolution_time: 30min
---

# Missing EDPROOF_HMAC_SECRET in smoke deploy pipeline

## Problem

Smoke deploy succeeds (NixOS build, env file upload, nixos-rebuild) but `mailservice-api.service` crashes on startup with exit code 1 in ~14ms.
No application logs visible in CI — only systemd unit status showing `code=exited, status=1/FAILURE`.

## Root Cause

`config.Load()` requires `EDPROOF_HMAC_SECRET` (>= 32 bytes) at startup.
The smoke deploy workflow (`deploy-smoke.yml`) never included this variable in the env file it generates and uploads to `/var/lib/secrets/mailservice.env`.

The production deploy workflow (`deploy-production.yml`) had it — smoke was the gap.

Previous deploys appeared to work because a manually-placed `.env` file at `/var/lib/mailservice/.env` (the service's `WorkingDirectory`) provided the secret via `godotenv.Read()` fallback in `loadDotEnv()`.

## Solution

1. Added `EDPROOF_HMAC_SECRET` to three places in `deploy-smoke.yml`:

```yaml
# Env file generation (printf block)
printf 'EDPROOF_HMAC_SECRET=%s\n' "$EDPROOF_HMAC_SECRET"

# Environment mapping
EDPROOF_HMAC_SECRET: ${{ secrets.EDPROOF_HMAC_SECRET }}

# Validation check
for var in ... EDPROOF_HMAC_SECRET ...; do
```

2. Set the GitHub secret:
```bash
openssl rand -hex 32 | gh secret set EDPROOF_HMAC_SECRET --env smoke
```

3. Redeployed — service starts, health check passes.

## Prevention

When adding a new required config variable to `config.Load()`, update **all** deploy workflows — not just production.
Compare `deploy-smoke.yml` and `deploy-production.yml` env var lists to catch drift.

The validation step (`Validate required env vars on host`) only catches vars in its check list.
If a new required var is added to `config.Load()` but not to the validation list, the deploy will pass validation but the service will crash.

**Checklist for new required config vars:**
- [ ] Add to `config.Load()` with validation
- [ ] Add to `deploy-production.yml` (env generation + env mapping + validation)
- [ ] Add to `deploy-smoke.yml` (env generation + env mapping + validation)
- [ ] Set the GitHub secret/variable in both environments
