#!/usr/bin/env bash
#
# Check recent Polar webhook deliveries for debugging.
# Requires: POLAR_TOKEN, POLAR_API

set -euo pipefail

POLAR_TOKEN="${POLAR_TOKEN:?POLAR_TOKEN is required}"
POLAR_API="${POLAR_API:-https://sandbox-api.polar.sh}"
WEBHOOK_ID="${WEBHOOK_ID:-}"

TMPBODY="$(mktemp)"
trap 'rm -f "$TMPBODY"' EXIT

# If no webhook ID, list endpoints first
if [ -z "$WEBHOOK_ID" ]; then
  curl -s --max-time 15 -L \
    -H "Authorization: Bearer $POLAR_TOKEN" \
    -o "$TMPBODY" \
    "${POLAR_API}/v1/webhooks/endpoints?limit=10"

  echo "=== Webhook Endpoints ==="
  jq -r '.items[] | "  \(.id) \(.url) events=[\(.events | join(", "))]"' "$TMPBODY"
  echo ""

  WEBHOOK_ID=$(jq -r '.items[0].id // empty' "$TMPBODY")
  if [ -z "$WEBHOOK_ID" ]; then
    echo "No webhooks found"
    exit 0
  fi
fi

# List recent deliveries
echo "=== Recent Deliveries (webhook $WEBHOOK_ID) ==="
curl -s --max-time 15 -L \
  -H "Authorization: Bearer $POLAR_TOKEN" \
  -o "$TMPBODY" \
  "${POLAR_API}/v1/webhooks/deliveries?webhook_endpoint_id=$WEBHOOK_ID&limit=10"

jq -r '.items[] | "  \(.created_at) event=\(.webhook_event.event // "?") status=\(.http_code // "pending") success=\(.succeeded)"' "$TMPBODY" 2>/dev/null || jq . "$TMPBODY"
