#!/usr/bin/env bash
set -euo pipefail

WORK_DIR=${NIXOPS_WORK_DIR:-$PWD}
DEPLOYMENT=${NIXOPS_DEPLOYMENT:-mailservice-truevipaccess}
NETWORK_FILE=${NIXOPS_NETWORK_FILE:-"$WORK_DIR/nixops/default.nix"}

if [ ! -f "$NETWORK_FILE" ]; then
  echo "NixOps network file not found: $NETWORK_FILE" >&2
  echo "Run this from the repo root or set NIXOPS_WORK_DIR / NIXOPS_NETWORK_FILE." >&2
  exit 1
fi

cd "$WORK_DIR"

exec nix shell nixpkgs#nixops_unstable_minimal --command \
  nixops create -d "$DEPLOYMENT"
