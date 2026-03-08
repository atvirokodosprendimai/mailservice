#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Create the OpenTofu state bucket in Hetzner Object Storage.

Required environment:
  HETZNER_OBJECT_STORAGE_ACCESS_KEY
  HETZNER_OBJECT_STORAGE_SECRET_KEY
  TOFU_STATE_BUCKET
  TOFU_STATE_REGION      one of: fsn1, nbg1, hel1

Optional environment:
  TOFU_STATE_ENDPOINT    defaults to https://<region>.your-objectstorage.com
  AWS_PROFILE            defaults to hetzner-object-storage

Prerequisites:
  - aws CLI v2
  - S3 credentials already created manually in Hetzner Console

Example:
  export HETZNER_OBJECT_STORAGE_ACCESS_KEY=...
  export HETZNER_OBJECT_STORAGE_SECRET_KEY=...
  export TOFU_STATE_BUCKET=mailservice-tofu-state
  export TOFU_STATE_REGION=fsn1
  ./ops/object-storage-bootstrap/create_tofu_state_bucket.sh
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

command -v aws >/dev/null 2>&1 || {
  echo "aws CLI is required" >&2
  exit 1
}

require HETZNER_OBJECT_STORAGE_ACCESS_KEY
require HETZNER_OBJECT_STORAGE_SECRET_KEY
require TOFU_STATE_BUCKET
require TOFU_STATE_REGION

case "${TOFU_STATE_REGION}" in
  fsn1|nbg1|hel1) ;;
  *)
    echo "TOFU_STATE_REGION must be one of: fsn1, nbg1, hel1" >&2
    exit 1
    ;;
esac

export AWS_ACCESS_KEY_ID="${HETZNER_OBJECT_STORAGE_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${HETZNER_OBJECT_STORAGE_SECRET_KEY}"
export AWS_EC2_METADATA_DISABLED="true"
AWS_PROFILE="${AWS_PROFILE:-hetzner-object-storage}"
TOFU_STATE_ENDPOINT="${TOFU_STATE_ENDPOINT:-https://${TOFU_STATE_REGION}.your-objectstorage.com}"

echo "Checking bucket ${TOFU_STATE_BUCKET} on ${TOFU_STATE_ENDPOINT}..."
if aws --profile "${AWS_PROFILE}" \
  --endpoint-url "${TOFU_STATE_ENDPOINT}" \
  s3api head-bucket \
  --bucket "${TOFU_STATE_BUCKET}" >/dev/null 2>&1; then
  echo "Bucket already exists: ${TOFU_STATE_BUCKET}"
else
  echo "Creating bucket: ${TOFU_STATE_BUCKET}"
  aws --profile "${AWS_PROFILE}" \
    --endpoint-url "${TOFU_STATE_ENDPOINT}" \
    s3api create-bucket \
    --bucket "${TOFU_STATE_BUCKET}"
fi

cat <<EOF

Bucket ready.

GitHub secrets to set:
  TOFU_STATE_BUCKET=${TOFU_STATE_BUCKET}
  TOFU_STATE_REGION=${TOFU_STATE_REGION}
  TOFU_STATE_ENDPOINT=${TOFU_STATE_ENDPOINT}
  TOFU_STATE_ACCESS_KEY=<your access key>
  TOFU_STATE_SECRET_KEY=<your secret key>
EOF
