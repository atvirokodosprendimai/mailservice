#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
DEPLOYMENT=${NIXOPS_DEPLOYMENT:-mailservice-truevipaccess}
NETWORK_FILE=${NIXOPS_NETWORK_FILE:-"$ROOT_DIR/nixops/default.nix"}

cd "$ROOT_DIR"

exec nix shell nixpkgs#nixops --command \
  nixops create "$NETWORK_FILE" -d "$DEPLOYMENT"
