#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Push Hetzner Object Storage OpenTofu backend values into GitHub repo secrets.

Required environment:
  GITHUB_REPOSITORY                  e.g. atvirokodosprendimai/mailservice
  TOFU_STATE_BUCKET
  TOFU_STATE_REGION                  one of: fsn1, nbg1, hel1
  TOFU_STATE_ENDPOINT                e.g. https://fsn1.your-objectstorage.com
  HETZNER_OBJECT_STORAGE_ACCESS_KEY
  HETZNER_OBJECT_STORAGE_SECRET_KEY

Prerequisites:
  - gh authenticated with repo admin access

Example:
  export GITHUB_REPOSITORY=atvirokodosprendimai/mailservice
  export TOFU_STATE_BUCKET=mailservice-tofu-state
  export TOFU_STATE_REGION=fsn1
  export TOFU_STATE_ENDPOINT=https://fsn1.your-objectstorage.com
  export HETZNER_OBJECT_STORAGE_ACCESS_KEY=...
  export HETZNER_OBJECT_STORAGE_SECRET_KEY=...
  ./ops/object-storage-bootstrap/set_github_tofu_state_secrets.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

require() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required environment variable: $name" >&2
    exit 1
  fi
}

command -v gh >/dev/null 2>&1 || {
  echo "gh CLI is required" >&2
  exit 1
}

require GITHUB_REPOSITORY
require TOFU_STATE_BUCKET
require TOFU_STATE_REGION
require TOFU_STATE_ENDPOINT
require HETZNER_OBJECT_STORAGE_ACCESS_KEY
require HETZNER_OBJECT_STORAGE_SECRET_KEY

printf '%s' "${TOFU_STATE_BUCKET}" | gh secret set TOFU_STATE_BUCKET --repo "${GITHUB_REPOSITORY}"
printf '%s' "${TOFU_STATE_REGION}" | gh secret set TOFU_STATE_REGION --repo "${GITHUB_REPOSITORY}"
printf '%s' "${TOFU_STATE_ENDPOINT}" | gh secret set TOFU_STATE_ENDPOINT --repo "${GITHUB_REPOSITORY}"
printf '%s' "${HETZNER_OBJECT_STORAGE_ACCESS_KEY}" | gh secret set TOFU_STATE_ACCESS_KEY --repo "${GITHUB_REPOSITORY}"
printf '%s' "${HETZNER_OBJECT_STORAGE_SECRET_KEY}" | gh secret set TOFU_STATE_SECRET_KEY --repo "${GITHUB_REPOSITORY}"

echo "GitHub OpenTofu state secrets updated for ${GITHUB_REPOSITORY}"
