#!/usr/bin/env bash
set -euo pipefail

DEPLOYMENT=${NIXOPS_DEPLOYMENT:-mailservice-truevipaccess}

exec nix shell nixpkgs#nixops --command \
  nixops rollback -d "$DEPLOYMENT"
