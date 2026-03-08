# Hetzner Object Storage Bootstrap

Use this when you want a real S3-compatible backend for OpenTofu without bootstrapping your own Garage cluster first.

## Important Constraint

You cannot bootstrap Hetzner Object Storage entirely from `HCLOUD_API`.

Per Hetzner's Object Storage docs:
- S3 credentials are created in Hetzner Console, not via the S3 API
- bucket and object operations happen via the S3-compatible API after you have those credentials

Sources:
- https://docs.hetzner.com/storage/object-storage/faq/general
- https://docs.hetzner.com/storage/object-storage/getting-started/using-s3-api-tools/

So the correct split is:
- `HCLOUD_API` continues to manage Hetzner Cloud resources through OpenTofu
- Hetzner Object Storage access key and secret key are created once in Hetzner Console
- shell/Ansible then create the OpenTofu bucket and push the values into GitHub secrets

## Endpoint Map

Hetzner Object Storage endpoints are location-bound:

- `fsn1.your-objectstorage.com`
- `nbg1.your-objectstorage.com`
- `hel1.your-objectstorage.com`

Source:
- https://docs.hetzner.com/storage/object-storage/overview/

## Prerequisites

1. Create Object Storage credentials in the correct Hetzner project.
2. Save the access key and secret key immediately.
3. Install:
   - `aws`
   - `gh`
   - optional: `ansible`

## Shell Path

Set the required environment:

```bash
export GITHUB_REPOSITORY=atvirokodosprendimai/mailservice
export HETZNER_OBJECT_STORAGE_ACCESS_KEY=...
export HETZNER_OBJECT_STORAGE_SECRET_KEY=...
export TOFU_STATE_BUCKET=mailservice-tofu-state
export TOFU_STATE_REGION=fsn1
export TOFU_STATE_ENDPOINT=https://fsn1.your-objectstorage.com
```

Create the bucket:

```bash
./ops/object-storage-bootstrap/create_tofu_state_bucket.sh
```

Push the OpenTofu backend secrets into GitHub:

```bash
./ops/object-storage-bootstrap/set_github_tofu_state_secrets.sh
```

## Ansible Path

If you prefer one command:

```bash
ansible-playbook ops/object-storage-bootstrap/ansible/bootstrap_tofu_state.yml
```

The playbook runs locally and wraps the same shell scripts.

## Result

After this, the repo should have these secrets populated:

- `TOFU_STATE_BUCKET`
- `TOFU_STATE_REGION`
- `TOFU_STATE_ENDPOINT`
- `TOFU_STATE_ACCESS_KEY`
- `TOFU_STATE_SECRET_KEY`

Then the existing `Hetzner OpenTofu` GitHub Actions workflow can use the current S3-compatible backend path as-is.

## Why This Is Better Than Bootstrapping Garage First

- managed S3-compatible storage
- no extra VM to secure and operate
- no self-hosted object-storage bootstrap cycle
- matches the repo's current OpenTofu workflow design directly
