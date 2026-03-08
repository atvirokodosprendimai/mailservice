# Hetzner NixOS Snapshot Builder

## Goal

Produce a Hetzner snapshot id for a fresh NixOS host so the existing OpenTofu
workflow can provision a NixOS target with:

- `image=<snapshot id>`
- `bootstrap_mode=none`

## Shape

The builder flow uses a temporary Ubuntu VM, installs NixOS onto it with
`nixos-infect`, then snapshots the powered-off disk and deletes the builder.

This is a bootstrap path, not the long-term deployment model.

## Script

Use:

- [ops/hetzner-nixos-image/build_snapshot.sh](../ops/hetzner-nixos-image/build_snapshot.sh)

Required env:

- `HCLOUD_TOKEN`
- `PUBLIC_KEY_FILE`
- `PRIVATE_KEY_FILE`

Optional env:

- `NAME_PREFIX` default `mailservice-nixos`
- `LOCATION` default `hel1`
- `SERVER_TYPE` default `cpx22`
- `NIXOS_CHANNEL` default `nixos-24.11`
- `SNAPSHOT_NAME`
- `KEEP_BUILDER=true` if you want to keep the temporary VM for debugging

Example:

```bash
HCLOUD_TOKEN=... \
PUBLIC_KEY_FILE=~/.ssh/id_ed25519.pub \
PRIVATE_KEY_FILE=~/.ssh/id_ed25519 \
./ops/hetzner-nixos-image/build_snapshot.sh
```

Expected output:

```text
snapshot_name=mailservice-nixos-nixos-24.11-20260308220000
snapshot_id=123456789
```

## Next Step

Run the existing Hetzner OpenTofu workflow with:

- `image=<snapshot_id>`
- `bootstrap_mode=none`

That creates the fresh NixOS target VM without injecting the Ubuntu Docker
bootstrap.
