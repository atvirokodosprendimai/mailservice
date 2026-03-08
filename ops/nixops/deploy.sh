#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
DEPLOYMENT=${NIXOPS_DEPLOYMENT:-mailservice-truevipaccess}
NETWORK_FILE=${NIXOPS_NETWORK_FILE:-"$ROOT_DIR/nixops/default.nix"}

cd "$ROOT_DIR"

if ! nix shell nixpkgs#nixops --command nixops info -d "$DEPLOYMENT" >/dev/null 2>&1; then
  nix shell nixpkgs#nixops --command nixops create "$NETWORK_FILE" -d "$DEPLOYMENT"
fi

exec nix shell nixpkgs#nixops --command \
  nixops deploy -d "$DEPLOYMENT"
