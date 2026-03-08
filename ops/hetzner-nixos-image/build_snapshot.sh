#!/usr/bin/env bash
set -euo pipefail

if ! command -v hcloud >/dev/null 2>&1; then
  echo "hcloud CLI is required." >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required." >&2
  exit 1
fi

if [ -z "${HCLOUD_TOKEN:-}" ]; then
  echo "HCLOUD_TOKEN is required." >&2
  exit 1
fi

if [ -z "${PUBLIC_KEY_FILE:-}" ] || [ ! -f "${PUBLIC_KEY_FILE}" ]; then
  echo "PUBLIC_KEY_FILE must point to an existing SSH public key file." >&2
  exit 1
fi

if [ -z "${PRIVATE_KEY_FILE:-}" ] || [ ! -f "${PRIVATE_KEY_FILE}" ]; then
  echo "PRIVATE_KEY_FILE must point to an existing SSH private key file." >&2
  exit 1
fi

export HCLOUD_TOKEN

NAME_PREFIX=${NAME_PREFIX:-mailservice-nixos}
LOCATION=${LOCATION:-hel1}
SERVER_TYPE=${SERVER_TYPE:-cpx22}
CHANNEL=${NIXOS_CHANNEL:-nixos-24.11}
SERVER_NAME="${NAME_PREFIX}-builder-$(date +%s)"
SSH_KEY_NAME="${SERVER_NAME}-ssh"
SNAPSHOT_NAME=${SNAPSHOT_NAME:-"${NAME_PREFIX}-${CHANNEL}-$(date +%Y%m%d%H%M%S)"}
KEEP_BUILDER=${KEEP_BUILDER:-false}
CREATED_SSH_KEY=false
PUBLIC_KEY_CONTENT=$(<"$PUBLIC_KEY_FILE")

cleanup() {
  if [ "${KEEP_BUILDER}" != "true" ] && [ -n "${SERVER_NAME:-}" ]; then
    hcloud server delete "$SERVER_NAME" >/dev/null 2>&1 || true
  fi
  if [ "${CREATED_SSH_KEY}" = "true" ] && [ -n "${SSH_KEY_NAME:-}" ]; then
    hcloud ssh-key delete "$SSH_KEY_NAME" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

EXISTING_SSH_KEY_NAME=$(hcloud ssh-key list -o json | jq -r --arg key "$PUBLIC_KEY_CONTENT" '.[] | select(.public_key == $key) | .name' | head -n1)
if [ -n "$EXISTING_SSH_KEY_NAME" ]; then
  SSH_KEY_NAME="$EXISTING_SSH_KEY_NAME"
  echo "Reusing existing SSH key ${SSH_KEY_NAME}"
else
  echo "Creating temporary SSH key ${SSH_KEY_NAME}"
  hcloud ssh-key create --name "$SSH_KEY_NAME" --public-key-from-file "$PUBLIC_KEY_FILE" >/dev/null
  CREATED_SSH_KEY=true
fi

echo "Creating temporary Ubuntu builder ${SERVER_NAME}"
hcloud server create \
  --name "$SERVER_NAME" \
  --image ubuntu-24.04 \
  --type "$SERVER_TYPE" \
  --location "$LOCATION" \
  --ssh-key "$SSH_KEY_NAME" >/dev/null

SERVER_IP=$(hcloud server describe "$SERVER_NAME" -o json | jq -r '.public_net.ipv4.ip')
if [ -z "$SERVER_IP" ] || [ "$SERVER_IP" = "null" ]; then
  echo "Failed to determine builder IP." >&2
  exit 1
fi

echo "Waiting for SSH on ${SERVER_IP}"
for attempt in $(seq 1 30); do
  if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -i "$PRIVATE_KEY_FILE" "root@${SERVER_IP}" true >/dev/null 2>&1; then
    break
  fi
  if [ "$attempt" -eq 30 ]; then
    echo "Builder SSH did not become ready." >&2
    exit 1
  fi
  sleep 5
done

echo "Running nixos-infect on ${SERVER_IP}"
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$PRIVATE_KEY_FILE" "root@${SERVER_IP}" \
  "curl -L https://raw.githubusercontent.com/elitak/nixos-infect/master/nixos-infect | NIX_CHANNEL=${CHANNEL} bash"

echo "Waiting for host to reboot into NixOS"
sleep 20
for attempt in $(seq 1 60); do
  if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o BatchMode=yes -i "$PRIVATE_KEY_FILE" "root@${SERVER_IP}" 'grep -q NixOS /etc/os-release' >/dev/null 2>&1; then
    break
  fi
  if [ "$attempt" -eq 60 ]; then
    echo "Builder did not come back as NixOS." >&2
    exit 1
  fi
  sleep 10
done

echo "Powering off ${SERVER_NAME} for snapshot consistency"
hcloud server poweroff "$SERVER_NAME" >/dev/null
for attempt in $(seq 1 30); do
  STATUS=$(hcloud server describe "$SERVER_NAME" -o json | jq -r '.status')
  if [ "$STATUS" = "off" ]; then
    break
  fi
  if [ "$attempt" -eq 30 ]; then
    echo "Server did not power off cleanly." >&2
    exit 1
  fi
  sleep 5
done

echo "Creating snapshot ${SNAPSHOT_NAME}"
IMAGE_JSON=$(hcloud server create-image "$SERVER_NAME" --type snapshot --description "$SNAPSHOT_NAME" -o json)
IMAGE_ID=$(printf '%s' "$IMAGE_JSON" | jq -r '.image.id // .action.resources[]? | select(.type=="image") | .id' | head -n1)

if [ -z "$IMAGE_ID" ] || [ "$IMAGE_ID" = "null" ]; then
  echo "Failed to determine snapshot id." >&2
  printf '%s\n' "$IMAGE_JSON" >&2
  exit 1
fi

echo "snapshot_name=${SNAPSHOT_NAME}"
echo "snapshot_id=${IMAGE_ID}"
