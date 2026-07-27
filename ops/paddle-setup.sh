#!/usr/bin/env bash
#
# Provision Paddle-side resources (product, price, discount, webhook
# notification destination) for the mailservice subscription, and write
# retrieved secrets straight to GitHub environment secrets.
#
# Paddle does NOT expose an API to create a seller account, submit
# business-category review, submit/approve a payment-link domain, or
# mint additional API keys -- those are Dashboard-only, human actions.
# This script does not perform them. Before running it for real:
#
#   1. A Paddle seller account must exist and (for --env live) its
#      business-category review must be APPROVED, not merely submitted.
#      Anonymous-mailbox services can draw extended review or an
#      outright decline; treat this as the first action of the unit,
#      done well before this script is ever run.
#   2. For --env live, the default payment-link domain must be
#      submitted AND approved in the Paddle Dashboard (sandbox needs
#      neither of these gates).
#   3. A full-access "setup" API key exists (Dashboard > Developer
#      Tools > Authentication) and is passed in as PADDLE_SETUP_API_KEY.
#      Because Paddle has no API to mint additional keys, the
#      minimum-permission runtime key required by KTD11
#      (transaction.write, transaction.read, subscription.read,
#      discount.read -- no customer write, no refund, no product/price
#      write) must be created by hand in the Dashboard with exactly
#      that permission set, then passed in as PADDLE_RUNTIME_API_KEY
#      for this script to store as a secret.
#
# Requires:
#   PADDLE_SETUP_API_KEY   Full-access key used only to run this script.
#   PADDLE_ENVIRONMENT     "sandbox" or "live".
#   WEBHOOK_URL            Destination URL for the Paddle webhook.
#   MAILBOX_UNIT_PRICE_CENTS  Price in minor units (e.g. cents) for the
#                           recurring price. Not defaulted on purpose --
#                           the plan flags MAILBOX_PRICE_CENTS=100 vs.
#                           the 299 template figure as an unresolved
#                           product-owner decision. Pass the resolved
#                           value explicitly; this script refuses to
#                           guess.
# Optional:
#   PADDLE_API              Override the API base URL (auto-selected
#                            from PADDLE_ENVIRONMENT otherwise).
#   GH_ENV                  GitHub environment secrets are written to.
#                            Defaults to "smoke" for sandbox, "production"
#                            for live.
#   PADDLE_RUNTIME_API_KEY  If set, stored as the PADDLE_API_KEY secret
#                            after validating its prefix matches
#                            PADDLE_ENVIRONMENT.
#   PADDLE_APPROVALS_CONFIRMED  Must be "yes" to run against --env live
#                            (guards against provisioning a live price
#                            before the human approval gates above are
#                            actually cleared). Not required for sandbox.
#
# Secret handling (KTD11): the webhook endpoint_secret_key and the
# runtime API key are never echoed to stdout or logged -- they are
# piped straight into `gh secret set --env "$GH_ENV"`. Only non-secret
# IDs (product, price, discount, notification destination) and the
# pinned runtime-key permission set are printed.
#
# Idempotent: safe to re-run. Existing product/price/discount/webhook
# resources are detected by name/code/URL and reused rather than
# duplicated.

set -euo pipefail

print_usage() {
  cat <<'EOF'
Usage: ops/paddle-setup.sh

Environment variables (see script header for full detail):
  Required: PADDLE_SETUP_API_KEY, PADDLE_ENVIRONMENT, WEBHOOK_URL,
            MAILBOX_UNIT_PRICE_CENTS
  Optional: PADDLE_API, GH_ENV, PADDLE_RUNTIME_API_KEY,
            PADDLE_APPROVALS_CONFIRMED (required "yes" for --env live)

Provisions a Paddle product, price, the OPENCLAWS discount, and a
webhook notification destination; writes secrets directly to
`gh secret set --env`. Never prints secret-shaped values.
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  print_usage
  exit 0
fi

PADDLE_SETUP_API_KEY="${PADDLE_SETUP_API_KEY:?PADDLE_SETUP_API_KEY is required (full-access Dashboard key, used only to run this script)}"
PADDLE_ENVIRONMENT="${PADDLE_ENVIRONMENT:?PADDLE_ENVIRONMENT is required: sandbox or live}"
WEBHOOK_URL="${WEBHOOK_URL:?WEBHOOK_URL is required}"
MAILBOX_UNIT_PRICE_CENTS="${MAILBOX_UNIT_PRICE_CENTS:?MAILBOX_UNIT_PRICE_CENTS is required -- this is a product-owner pricing decision (see MAILBOX_PRICE_CENTS=100 vs. template 299 in the plan), not something this script defaults}"

case "$PADDLE_ENVIRONMENT" in
  sandbox)
    PADDLE_API="${PADDLE_API:-https://sandbox-api.paddle.com}"
    GH_ENV="${GH_ENV:-smoke}"
    RUNTIME_KEY_PREFIX="pdl_sdbx_apikey_"
    ;;
  live)
    PADDLE_API="${PADDLE_API:-https://api.paddle.com}"
    GH_ENV="${GH_ENV:-production}"
    RUNTIME_KEY_PREFIX="pdl_live_apikey_"
    if [ "${PADDLE_APPROVALS_CONFIRMED:-}" != "yes" ]; then
      echo "ERROR: --env live requires PADDLE_APPROVALS_CONFIRMED=yes." >&2
      echo "This confirms a human has verified, not just submitted:" >&2
      echo "  1. Paddle seller/business-category approval is GRANTED." >&2
      echo "  2. The default payment-link domain is GRANTED." >&2
      echo "Provisioning a live price before those gates clear risks an" >&2
      echo "immutable price nobody can transact against." >&2
      exit 1
    fi
    ;;
  *)
    echo "ERROR: PADDLE_ENVIRONMENT must be 'sandbox' or 'live', got '$PADDLE_ENVIRONMENT'" >&2
    exit 1
    ;;
esac

PRODUCT_NAME="Mailbox Subscription"
DISCOUNT_CODE="OPENCLAWS"
WEBHOOK_DESCRIPTION="mailservice ${PADDLE_ENVIRONMENT} webhook"
SUBSCRIBED_EVENTS='[
  "transaction.completed",
  "transaction.paid",
  "transaction.updated",
  "subscription.created",
  "subscription.activated",
  "subscription.updated",
  "subscription.canceled",
  "subscription.past_due"
]'

TMPBODY="$(mktemp)"
trap 'rm -f "$TMPBODY"' EXIT

paddle_get() {
  local path="$1"
  curl -s --max-time 15 -L \
    -H "Authorization: Bearer $PADDLE_SETUP_API_KEY" \
    -o "$TMPBODY" -w '%{http_code}' \
    "${PADDLE_API}${path}"
}

paddle_post() {
  local path="$1" body="$2"
  curl -s --max-time 15 -L -X POST \
    -H "Authorization: Bearer $PADDLE_SETUP_API_KEY" \
    -H "Content-Type: application/json" \
    -d "$body" \
    -o "$TMPBODY" -w '%{http_code}' \
    "${PADDLE_API}${path}"
}

require_ok() {
  local http_code="$1" action="$2"
  case "$http_code" in
    200|201) ;;
    *)
      echo "ERROR: $action returned HTTP $http_code" >&2
      cat "$TMPBODY" >&2
      exit 1
      ;;
  esac
}

# --- Product -----------------------------------------------------------

HTTP_CODE=$(paddle_get "/products?per_page=100")
require_ok "$HTTP_CODE" "list products"
PRODUCT_ID=$(jq -r --arg name "$PRODUCT_NAME" '.data[]? | select(.name == $name) | .id' "$TMPBODY" | head -1)

if [ -n "$PRODUCT_ID" ]; then
  echo "Product already exists: $PRODUCT_ID" >&2
else
  echo "Creating product: $PRODUCT_NAME" >&2
  HTTP_CODE=$(paddle_post "/products" "$(jq -n --arg name "$PRODUCT_NAME" '{
    name: $name,
    tax_category: "saas",
    description: "Anonymous agent mailbox subscription"
  }')")
  require_ok "$HTTP_CODE" "create product"
  PRODUCT_ID=$(jq -r '.data.id' "$TMPBODY")
  echo "Created product: $PRODUCT_ID" >&2
fi

# --- Price ---------------------------------------------------------------

HTTP_CODE=$(paddle_get "/prices?product_id=${PRODUCT_ID}&per_page=100")
require_ok "$HTTP_CODE" "list prices"
PRICE_ID=$(jq -r --arg amount "$MAILBOX_UNIT_PRICE_CENTS" \
  '.data[]? | select(.unit_price.amount == $amount and .billing_cycle.interval == "month" and .billing_cycle.frequency == 1) | .id' \
  "$TMPBODY" | head -1)

if [ -n "$PRICE_ID" ]; then
  echo "Price already exists: $PRICE_ID" >&2
else
  echo "Creating price: ${MAILBOX_UNIT_PRICE_CENTS} EUR-minor-units/month" >&2
  HTTP_CODE=$(paddle_post "/prices" "$(jq -n \
    --arg product_id "$PRODUCT_ID" \
    --arg amount "$MAILBOX_UNIT_PRICE_CENTS" \
    '{
      product_id: $product_id,
      description: "Mailbox subscription monthly price",
      unit_price: { amount: $amount, currency_code: "EUR" },
      billing_cycle: { interval: "month", frequency: 1 },
      tax_mode: "account_setting"
    }')")
  require_ok "$HTTP_CODE" "create price"
  PRICE_ID=$(jq -r '.data.id' "$TMPBODY")
  echo "Created price: $PRICE_ID" >&2
fi

# --- Discount (OPENCLAWS) --------------------------------------------------

HTTP_CODE=$(paddle_get "/discounts?per_page=100")
require_ok "$HTTP_CODE" "list discounts"
DISCOUNT_ID=$(jq -r --arg code "$DISCOUNT_CODE" '.data[]? | select(.code == $code) | .id' "$TMPBODY" | head -1)

if [ -n "$DISCOUNT_ID" ]; then
  echo "Discount already exists: $DISCOUNT_ID" >&2
else
  echo "Creating discount: $DISCOUNT_CODE" >&2
  HTTP_CODE=$(paddle_post "/discounts" "$(jq -n --arg code "$DISCOUNT_CODE" '{
    description: "OpenClaw community gift (3 months free)",
    type: "percentage",
    amount: "100",
    code: $code,
    recur: false,
    usage_limit: 23,
    enabled_for_checkout: true,
    status: "active"
  }')")
  require_ok "$HTTP_CODE" "create discount"
  DISCOUNT_ID=$(jq -r '.data.id' "$TMPBODY")
  echo "Created discount: $DISCOUNT_ID" >&2
fi

# --- Webhook notification destination -------------------------------------

HTTP_CODE=$(paddle_get "/notification-settings")
require_ok "$HTTP_CODE" "list notification destinations"
DESTINATION_ID=$(jq -r --arg url "$WEBHOOK_URL" '.data[]? | select(.destination == $url) | .id' "$TMPBODY" | head -1)

WEBHOOK_SECRET=""
if [ -n "$DESTINATION_ID" ]; then
  echo "Webhook destination already exists: $DESTINATION_ID" >&2
  HTTP_CODE=$(paddle_get "/notification-settings/${DESTINATION_ID}")
  require_ok "$HTTP_CODE" "get notification destination"
  WEBHOOK_SECRET=$(jq -r '.data.endpoint_secret_key // empty' "$TMPBODY")
  if [ -z "$WEBHOOK_SECRET" ]; then
    echo "Could not retrieve secret for existing destination; leaving PADDLE_WEBHOOK_SECRET untouched." >&2
  fi
else
  echo "Creating webhook destination: $WEBHOOK_URL" >&2
  HTTP_CODE=$(paddle_post "/notification-settings" "$(jq -n \
    --arg url "$WEBHOOK_URL" \
    --arg description "$WEBHOOK_DESCRIPTION" \
    --argjson events "$SUBSCRIBED_EVENTS" \
    '{
      description: $description,
      destination: $url,
      type: "url",
      subscribed_events: $events,
      active: true,
      api_version: 1,
      include_sensitive_data: false
    }')")
  require_ok "$HTTP_CODE" "create notification destination"
  DESTINATION_ID=$(jq -r '.data.id' "$TMPBODY")
  WEBHOOK_SECRET=$(jq -r '.data.endpoint_secret_key // empty' "$TMPBODY")
  echo "Created webhook destination: $DESTINATION_ID" >&2
fi

# --- Write secrets directly to GitHub (never echoed) ----------------------

if [ -n "$WEBHOOK_SECRET" ]; then
  printf '%s' "$WEBHOOK_SECRET" | gh secret set PADDLE_WEBHOOK_SECRET --env "$GH_ENV"
  echo "Stored PADDLE_WEBHOOK_SECRET in GitHub environment '$GH_ENV'." >&2
fi

if [ -n "${PADDLE_RUNTIME_API_KEY:-}" ]; then
  case "$PADDLE_RUNTIME_API_KEY" in
    "${RUNTIME_KEY_PREFIX}"*) ;;
    *)
      echo "ERROR: PADDLE_RUNTIME_API_KEY does not match PADDLE_ENVIRONMENT=$PADDLE_ENVIRONMENT: key must start with '$RUNTIME_KEY_PREFIX'" >&2
      exit 1
      ;;
  esac
  printf '%s' "$PADDLE_RUNTIME_API_KEY" | gh secret set PADDLE_API_KEY --env "$GH_ENV"
  echo "Stored PADDLE_API_KEY in GitHub environment '$GH_ENV'." >&2
else
  echo "PADDLE_RUNTIME_API_KEY not provided -- skipped storing PADDLE_API_KEY." >&2
  echo "Create it by hand in the Paddle Dashboard with exactly this permission set," >&2
  echo "then re-run with PADDLE_RUNTIME_API_KEY set:" >&2
  echo "  transaction.write, transaction.read, subscription.read, discount.read" >&2
fi

# --- Non-secret summary -----------------------------------------------------

echo ""
echo "=== Provisioned resources (${PADDLE_ENVIRONMENT}) ==="
echo "Product ID:              $PRODUCT_ID"
echo "Price ID:                $PRICE_ID"
echo "Discount ID:             $DISCOUNT_ID"
echo "Webhook destination ID:  $DESTINATION_ID"
echo "Runtime key permissions: transaction.write, transaction.read, subscription.read, discount.read"
echo ""
echo "Set PADDLE_PRICE_ID=$PRICE_ID and PADDLE_GIFT_DISCOUNT_ID=$DISCOUNT_ID in config (U1)."
