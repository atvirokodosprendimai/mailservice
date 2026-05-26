---
title: Hetzner OpenTofu workflow Cloudflare token rotation gap
date: 2026-05-26
category: integration-issues
module: nix/modules/mailservice-gitops.nix
problem_type: integration_issue
component: tooling
severity: high
symptoms:
  - "Hetzner OpenTofu workflow failed with `invalid value for api_token (API tokens must only contain characters a-z, A-Z, 0-9, hyphens and underscores)`"
  - "Substituting BEERPUB_CLOUDFLARE_API_TOKEN passed the auth charset gate but failed with `data.cloudflare_zones.domain.zones is empty list of object` (wrong Cloudflare account)"
  - "Fresh token scoped only to Zone Write authenticated and saw the zone but failed DNS record refresh with `Authentication error (10000)`"
  - "Failure was invisible from 2026-05-04 to 2026-05-26 because no path-filtered file was touched until commit 7404cca"
root_cause: missing_permission
resolution_type: config_change
tags:
  - cloudflare
  - opentofu
  - hetzner
  - github-actions
  - secret-rotation
  - dns
related_components:
  - development_workflow
  - authentication
---

# Hetzner OpenTofu workflow Cloudflare token rotation gap

## Problem

The Hetzner OpenTofu workflow began failing on every run after a Cloudflare API token consolidation deleted the `CLOUDFLARE_API_TOKEN` secret without updating the workflow reference. Recovering required three sequential fixes spanning secret rotation, account scoping, and Cloudflare's split permission model for Zone vs DNS operations.

## Symptoms

Three distinct failures surfaced over ~2.5 hours on 2026-05-26, each unmasking the next layer:

- **Stage 1 — empty token (run `26436937296`, 06:48 UTC):**
  ```
  Error: invalid value for api_token (API tokens must only contain
  characters a-z, A-Z, 0-9, hyphens and underscores)
  ```
  Workflow env showed `TF_VAR_cloudflare_api_token:` (empty) while `TF_VAR_hcloud_token: ***` resolved. The empty string failed Cloudflare provider's charset regex.

- **Stage 2 — wrong account (run `26442680101`, 09:00 UTC):**
  ```
  data.cloudflare_zones.domain: Read complete after 0s [id=d41d8cd98f00b204e9800998ecf8427e]
  Error: Invalid index
    zone_id = data.cloudflare_zones.domain.zones[0].id
  data.cloudflare_zones.domain.zones is empty list of object
  ```
  The `d41d8cd9…` ID is the MD5 of an empty string — Cloudflare returned zero zones because the token authenticated against an account that doesn't own `truevipaccess.com`.

- **Stage 3 — split permissions (run `26443233524`, 09:11 UTC):**
  ```
  data.cloudflare_zones.domain: Read complete after 3s [id=3a2d7a25150eb09836edc19d3efb3091]
  cloudflare_record.mail_a: Refreshing state... [id=1a08c1570b90568b9b9a57a225fe9f03]
  Error: Authentication error (10000)
    with cloudflare_record.mail_a, on main.tf line 104
  ```
  Zone now resolved (correct account), but record-level CRUD was denied — Zone Write alone doesn't grant DNS record edit rights.

Trigger context: commit `7404cca` touched `nix/modules/mailservice-gitops.nix`, which sits in the workflow's path-filter at `.github/workflows/hetzner-opentofu.yml:7`. An app-runtime systemd unit change retriggered the infra pipeline — exposing breakage that had been latent since the last successful run on 2026-05-04.

## What Didn't Work

### Attempt 1 — Secret rename substitution (commit `8ef3db9`)

Git archeology via `git log -S "CLOUDFLARE_API_TOKEN" -- .github/workflows/hetzner-opentofu.yml`:

| Commit / event | Date | What |
|---|---|---|
| `e9d667f` | 2026-03-08 | Cloudflare integration added with `secrets.CLOUDFLARE_TUNNEL_TOKEN` — wrong scope, hit error 6003 |
| `6d3168c` | 2026-03-09 | Switched to `secrets.CLOUDFLARE_API_TOKEN` |
| run `25298227654` | 2026-05-04 | Last successful run |
| between 05-04 and 05-26 | | `CLOUDFLARE_API_TOKEN` deleted from repo + all envs; `BEERPUB_CLOUDFLARE_API_TOKEN` created 2026-04-27 (looks like consolidation rename) |

The failing reference site:

```yaml
env:
  TF_VAR_cloudflare_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
```

The substitution applied:

```diff
- TF_VAR_cloudflare_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
+ TF_VAR_cloudflare_api_token: ${{ secrets.BEERPUB_CLOUDFLARE_API_TOKEN }}
```

(applied at three call sites in the workflow)

**Why it failed:** `BEERPUB_CLOUDFLARE_API_TOKEN` belonged to a different Cloudflare account. It authenticated cleanly but couldn't see the `truevipaccess.com` zone — Stage 2 surfaced as an empty zone list with the telltale `d41d8cd98f00b204e9800998ecf8427e` (MD5 of empty string) zone-data ID.

**Lesson learned:** Secret name similarity is not account identity. The `truevipaccess.com` zone lives in the `Illuminatus@group.lt` Cloudflare account, not BEERPUB's tenancy.

### Attempt 2 — New account-scoped token with Zone Write (commit `e4bbf2f`)

Created a fresh API token `mailservice-tofu` in the `Illuminatus@group.lt` account, scoped to the `truevipaccess.com` zone with **Zone → Zone → Write** permission. Set as repo secret `CLOUDFLARE_API_TOKEN`. Reverted the BEERPUB substitution so the workflow referenced `secrets.CLOUDFLARE_API_TOKEN` again at all three call sites.

**Why it failed:** Cloudflare splits Zone Write (zone-level settings: SSL, WAF, page rules) from DNS Edit (record CRUD) into **separate permission group entries**. With Zone Write only, `data.cloudflare_zones.domain` resolved correctly but `cloudflare_record.mail_a` state refresh threw `Authentication error (10000)` on read.

**Lesson learned:** Cloudflare's "Zone" permission group does not transitively grant DNS record CRUD. Each resource family Terraform manages needs its corresponding permission entry, and the names don't telegraph that — "Zone Write" sounds plenary.

## Solution

Three coordinated changes brought the pipeline green:

1. **Cloudflare-side token edit (no commit):** Added a second permission entry to the existing `mailservice-tofu` token: **Zone → DNS → Edit**, alongside the existing Zone → Zone → Write, both scoped to `truevipaccess.com`. Token value unchanged → no GH secret rotation needed.

2. **Workflow reference state (commit `e4bbf2f`, retained):** All three call sites reference the repo-level secret:
   ```yaml
   env:
     TF_VAR_cloudflare_api_token: ${{ secrets.CLOUDFLARE_API_TOKEN }}
   ```

3. **Verification (run `26443605529`, dispatched with `apply=false`):**
   - `validate` ✓
   - `plan` ✓ — state refresh on `cloudflare_record.*` succeeded (the exact operation that threw 10000 on the previous run)
   - auto-apply skipped (dispatch code path)

Final state:

- Repo-level GH secret `CLOUDFLARE_API_TOKEN` = fresh token from `Illuminatus@group.lt` account, scoped to `truevipaccess.com` zone with Zone Write + DNS Edit, no expiry.
- Workflow file at `.github/workflows/hetzner-opentofu.yml` references `secrets.CLOUDFLARE_API_TOKEN` in three places.
- Path filter at line 7 unchanged.

## Why This Works

Three independent root causes stacked into one observed outage:

**A. Dangling secret reference from incomplete consolidation.** The `CLOUDFLARE_API_TOKEN` repo secret was deleted (likely during the BEERPUB consolidation on 2026-04-27) but the workflow's three `${{ secrets.CLOUDFLARE_API_TOKEN }}` references remained. GitHub Actions silently resolves missing secrets to empty strings rather than erroring, so the breakage was invisible until the next trigger — three weeks later. This is the same family of failure as the smoke-env `POLAR_TOKEN` rotation gap earlier in this session (auto memory [claude]).

**B. Cloudflare's split permission model.** Cloudflare separates "Zone" operations (settings: SSL, WAF, page rules, zone-level config) from "DNS" operations (record CRUD: A, AAAA, MX, TXT, CNAME) into distinct permission group entries under the same Zone scope. Granting Zone Write alone makes `data.cloudflare_zones.domain` queries succeed (zone-level read) while denying any `cloudflare_record.*` refresh or apply. The provider error `Authentication error (10000)` doesn't name the missing permission group, masking the root cause.

**C. Workflow path-filter creates discovery latency.** `.github/workflows/hetzner-opentofu.yml:7` includes `nix/modules/mailservice-gitops.nix` in its paths trigger. That file is app-runtime systemd unit configuration, unrelated to Hetzner infra. The infra workflow therefore only ran when someone happened to touch a nix module — meaning a 3-week-old breakage looked brand new when commit `7404cca` (an `imap_login` shipper change) tripped it. Without that incidental trigger, the broken state would have persisted until the next deliberate infra change.

## Prevention

**Cloudflare token permission checklist** — apply when creating any Terraform-bound Cloudflare API token:

```
Required permissions for cloudflare provider managing DNS records:
  [ ] Zone -> Zone -> Read              (zone discovery via data source)
  [ ] Zone -> Zone -> Write             (zone settings: SSL, WAF, page rules)
  [ ] Zone -> DNS -> Edit               (DNS record CRUD - REQUIRED for cloudflare_record.*)
  [ ] Zone -> Page Rules -> Edit        (only if managing cloudflare_page_rule.*)
  [ ] Zone -> Workers Routes -> Edit    (only if managing cloudflare_worker_route.*)

Scoping:
  [ ] Resources: Include -> Specific zone -> <zone-name>  (NOT All zones)
  [ ] Client IP filter: leave blank (GH Actions runner IPs rotate)
  [ ] TTL: no expiry, OR calendared rotation with workflow secret update

Account verification before creating token:
  [ ] Confirm zone owner: dig +short NS <zone> -> trace to expected account
```

**Concrete preventive measures:**

1. **Out-of-band schedule trigger for infra workflows.** Add a `schedule:` cron (e.g., daily at 02:00 UTC, `apply=false`) to `.github/workflows/hetzner-opentofu.yml`. This decouples breakage detection from incidental path-filter triggers — the BEERPUB consolidation gap would have surfaced within 24h instead of 22 days. Same shape of fix as the Polar pulse pipeline blocker earlier in this session (auto memory [claude]).

2. **Tighten path filter to true infra surface.** Remove `nix/modules/mailservice-gitops.nix` and any app-runtime nix modules from the Hetzner workflow's path filter. The Hetzner workflow should trigger only on changes to `infra/opentofu/**` or the workflow file itself. App-runtime changes belong to the deploy pipeline, not the infra pipeline.

3. **Atomic secret consolidation / rename.** When renaming or consolidating a GitHub secret (e.g., `CLOUDFLARE_API_TOKEN` → `BEERPUB_CLOUDFLARE_API_TOKEN`), update workflow references **in the same commit** as the secret change. Use `gh secret list` and `grep -rn "secrets\.<OLD_NAME>" .github/` as a pre-flight check before deleting any secret.

4. **Lint workflows for dangling secret references.** A pre-commit or CI lint can compare `${{ secrets.X }}` references against `gh secret list` output and flag unresolved names. `docs/local-workflow-validation.md` is a candidate location to add this scan.

5. **Document token-to-account mapping.** Maintain a registry mapping each `<SECRET_NAME>` to `<cloudflare_account_email>` + `<zone>` + `<permissions>` so future engineers don't repeat the BEERPUB account-mismatch detour. The truevipaccess.com mapping is now pinned in auto memory (auto memory [claude]).

6. **Zero bugs andon on silent secret resolution.** When a workflow run shows `TF_VAR_*:` with an empty value next to a `***` masked sibling, that's the andon cord. Stop, audit all secret references, don't paper over with a one-line substitution. Stage 1's fix attempt skipped this step and cost a Stage 2 detour. (auto memory [claude] — zero bugs policy)

## Related Issues

- [`integration-issues/missing-edproof-hmac-secret-in-smoke-deploy.md`](../integration-issues/missing-edproof-hmac-secret-in-smoke-deploy.md) — sibling pattern: missing/broken secret reference in a CI workflow. Different exact failure (never-added vs deleted-rotation), shared prevention shape (validation checklist for required secrets across all workflows).
- [`docs/hetzner-cicd.md`](../../hetzner-cicd.md) — operational reference for Hetzner CI/CD secrets. Could gain a rotation/permission note for `CLOUDFLARE_*` tokens.
- [`docs/local-workflow-validation.md`](../../local-workflow-validation.md) — local pre-push validation for `hetzner-opentofu.yml`. Natural home for a dangling-secret-reference lint.
