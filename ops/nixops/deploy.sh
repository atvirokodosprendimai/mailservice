#!/usr/bin/env bash
set -euo pipefail

WORK_DIR=${NIXOPS_WORK_DIR:-$PWD}
DEPLOYMENT=${NIXOPS_DEPLOYMENT:-mailservice-truevipaccess}

if [ ! -f "$WORK_DIR/flake.nix" ]; then
  echo "flake.nix not found in $WORK_DIR" >&2
  echo "Run this from the repo root or set NIXOPS_WORK_DIR to the flake root." >&2
  exit 1
fi

cd "$WORK_DIR"

if ! info_error=$(nix shell nixpkgs#nixops_unstable_minimal --command nixops info -d "$DEPLOYMENT" 2>&1 >/dev/null); then
  case "$info_error" in
    *"does not exist"*|*"could not find"*)
      nix shell nixpkgs#nixops_unstable_minimal --command nixops create -d "$DEPLOYMENT"
      ;;
    *)
      printf '%s\n' "$info_error" >&2
      exit 1
      ;;
  esac
fi

exec nix shell nixpkgs#nixops_unstable_minimal --command \
  nixops deploy -d "$DEPLOYMENT" "$@"
