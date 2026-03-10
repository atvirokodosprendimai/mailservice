#!/usr/bin/env bash
#
# Create or update a Polar webhook endpoint for the smoke test server.
# Requires: POLAR_TOKEN, POLAR_API, WEBHOOK_URL
#
# Outputs the webhook secret to stdout.
# Idempotent: if a webhook for the URL already exists, prints its ID and exits.

set -euo pipefail

POLAR_TOKEN="${POLAR_TOKEN:?POLAR_TOKEN is required}"
POLAR_API="${POLAR_API:-https://sandbox-api.polar.sh}"
WEBHOOK_URL="${WEBHOOK_URL:?WEBHOOK_URL is required}"

TMPBODY="$(mktemp)"
trap 'rm -f "$TMPBODY"' EXIT

# List existing webhooks
HTTP_CODE=$(curl -s --max-time 15 -L \
  -H "Authorization: Bearer $POLAR_TOKEN" \
  -o "$TMPBODY" -w '%{http_code}' \
  "${POLAR_API}/v1/webhooks/endpoints?limit=100")

if [ "$HTTP_CODE" != "200" ]; then
  echo "ERROR: list webhooks returned HTTP $HTTP_CODE" >&2
  cat "$TMPBODY" >&2
  exit 1
fi

# Check if webhook for this URL already exists
EXISTING_ID=$(jq -r --arg url "$WEBHOOK_URL" '.items[] | select(.url == $url) | .id' "$TMPBODY" | head -1)

if [ -n "$EXISTING_ID" ]; then
  echo "Webhook already exists: $EXISTING_ID" >&2
  echo "URL: $WEBHOOK_URL" >&2

  # Get full details including events
  HTTP_CODE=$(curl -s --max-time 15 -L \
    -H "Authorization: Bearer $POLAR_TOKEN" \
    -o "$TMPBODY" -w '%{http_code}' \
    "${POLAR_API}/v1/webhooks/endpoints/$EXISTING_ID")

  if [ "$HTTP_CODE" = "200" ]; then
    EVENTS=$(jq -r '.events // [] | join(", ")' "$TMPBODY")
    echo "Events: $EVENTS" >&2

    # Ensure checkout.updated is in events
    if ! jq -e '.events | index("checkout.updated")' "$TMPBODY" >/dev/null 2>&1; then
      echo "Adding checkout.updated to webhook events..." >&2
      CURRENT_EVENTS=$(jq -c '.events + ["checkout.updated"] | unique' "$TMPBODY")
      curl -s --max-time 15 -L -X PUT \
        -H "Authorization: Bearer $POLAR_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg url "$WEBHOOK_URL" --argjson events "$CURRENT_EVENTS" '{url: $url, events: $events}')" \
        -o "$TMPBODY" -w '%{http_code}' \
        "${POLAR_API}/v1/webhooks/endpoints/$EXISTING_ID" >/dev/null
      echo "Updated events: $(jq -r '.events // [] | join(", ")' "$TMPBODY")" >&2
    fi

    SECRET=$(jq -r '.secret // empty' "$TMPBODY")
    if [ -n "$SECRET" ]; then
      echo "$SECRET"
      exit 0
    fi
  fi

  echo "Could not retrieve secret for existing webhook, resetting..." >&2
  HTTP_CODE=$(curl -s --max-time 15 -L -X POST \
    -H "Authorization: Bearer $POLAR_TOKEN" \
    -o "$TMPBODY" -w '%{http_code}' \
    "${POLAR_API}/v1/webhooks/endpoints/$EXISTING_ID/reset-secret")

  if [ "$HTTP_CODE" = "200" ]; then
    SECRET=$(jq -r '.secret // empty' "$TMPBODY")
    if [ -n "$SECRET" ]; then
      echo "$SECRET"
      exit 0
    fi
  fi

  echo "ERROR: could not get webhook secret" >&2
  cat "$TMPBODY" >&2
  exit 1
fi

# Create new webhook
echo "Creating webhook endpoint: $WEBHOOK_URL" >&2
HTTP_CODE=$(curl -s --max-time 15 -L -X POST \
  -H "Authorization: Bearer $POLAR_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg url "$WEBHOOK_URL" '{
    url: $url,
    format: "raw",
    events: ["checkout.updated"],
    secret: null
  }')" \
  -o "$TMPBODY" -w '%{http_code}' \
  "${POLAR_API}/v1/webhooks/endpoints")

if [ "$HTTP_CODE" != "200" ] && [ "$HTTP_CODE" != "201" ]; then
  echo "ERROR: create webhook returned HTTP $HTTP_CODE" >&2
  cat "$TMPBODY" >&2
  exit 1
fi

WEBHOOK_ID=$(jq -r '.id' "$TMPBODY")
SECRET=$(jq -r '.secret // empty' "$TMPBODY")

echo "Created webhook: $WEBHOOK_ID" >&2
echo "URL: $WEBHOOK_URL" >&2

if [ -n "$SECRET" ]; then
  echo "$SECRET"
else
  echo "WARNING: no secret in create response, fetching..." >&2
  HTTP_CODE=$(curl -s --max-time 15 -L \
    -H "Authorization: Bearer $POLAR_TOKEN" \
    -o "$TMPBODY" -w '%{http_code}' \
    "${POLAR_API}/v1/webhooks/endpoints/$WEBHOOK_ID")
  SECRET=$(jq -r '.secret // empty' "$TMPBODY")
  echo "$SECRET"
fi
