#!/usr/bin/env bash
#
# Periodic smoke test for mailservice.
# Designed to run every 5 minutes via cron or GitHub Actions schedule.
#
# Two modes:
#   1. Persistent key mode (default): reuses a key after first manual payment.
#   2. Auto-pay mode (--auto-pay): generates a fresh key each run and
#      auto-confirms via a headless browser against the Paddle sandbox
#      checkout overlay. Requires a free (100%-off) sandbox discount code,
#      since Paddle has no public REST API to confirm a transaction —
#      checkout completion is always browser/overlay-driven.
#
# Exit codes:
#   0 — all checks passed
#   1 — a check failed
#   2 — missing dependencies or bad arguments

set -euo pipefail

BASE_URL="${SMOKE_BASE_URL:-https://truevipaccess.com}"
BILLING_EMAIL="${SMOKE_BILLING_EMAIL:-smoke@truevipaccess.com}"
IMAP_HOST="${SMOKE_IMAP_HOST:-mail.truevipaccess.com}"
IMAP_PORT="${SMOKE_IMAP_PORT:-993}"
WORK_DIR="${SMOKE_WORK_DIR:-${TMPDIR:-/tmp}/mailservice-smoke-periodic}"
KEY_PATH="${SMOKE_KEY_PATH:-}"
VERBOSE="${SMOKE_VERBOSE:-0}"
AUTO_PAY="${SMOKE_AUTO_PAY:-0}"
ADMIN_API_KEY="${SMOKE_ADMIN_API_KEY:-}"
DISCOUNT_CODE="${SMOKE_DISCOUNT_CODE:-}"

usage() {
  cat <<'EOF'
Usage:
  ops/smoke-test-periodic.sh [options]

All options can also be set via SMOKE_* environment variables.

Options:
  --base-url URL          API base URL.           Env: SMOKE_BASE_URL
  --billing-email EMAIL   Billing email.          Env: SMOKE_BILLING_EMAIL
  --imap-host HOST        IMAP server hostname.   Env: SMOKE_IMAP_HOST
  --imap-port PORT        IMAP TLS port.          Env: SMOKE_IMAP_PORT
  --work-dir DIR          Persistent key storage. Env: SMOKE_WORK_DIR
  --key-path PATH         Ed25519 key path.       Env: SMOKE_KEY_PATH
  --auto-pay              Auto-confirm payment.   Env: SMOKE_AUTO_PAY=1
  --admin-api-key KEY     Admin API key.          Env: SMOKE_ADMIN_API_KEY
  --discount-code CODE    Paddle gift coupon code Env: SMOKE_DISCOUNT_CODE
                          applied at claim time.
  --verbose               Print details.          Env: SMOKE_VERBOSE=1
  --help                  Show this help.

Persistent key mode (default):
  Generates an Ed25519 key and claims a mailbox.
  If the mailbox is not yet paid, prints the payment URL and exits 1.
  After manual payment, all subsequent runs pass automatically.

Auto-pay mode (--auto-pay):
  Generates a fresh key each run and auto-confirms the checkout via a
  headless browser (Playwright) against the Paddle sandbox checkout overlay
  (ops/paddle-checkout-confirm.js). Requires --discount-code (or
  SMOKE_DISCOUNT_CODE) naming a 100%-off Paddle gift coupon provisioned for
  the smoke environment, so the checkout overlay needs no real card.
  Each run exercises the full claim → pay → activate → resolve → read flow.

Checks performed:
  1. GET  /healthz                — API is up
  2. POST /v1/mailboxes/claim     — claim succeeds
  3. Auto-pay (if enabled)        — confirm checkout via Paddle sandbox overlay
  4. POST /v1/access/resolve      — returns IMAP credentials
  5. IMAP LOGIN via TLS           — Dovecot authenticates
  6. POST /v1/imap/messages       — HTTP API returns messages
EOF
}

log() { echo "==> $*"; }
detail() { [[ "$VERBOSE" == "1" ]] && echo "    $*" || true; }
fail() { echo "FAIL: $*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 2; }
}

# Unified HTTP helper. Pass extra headers as additional arguments.
# Usage: http_json METHOD URL DATA BODY_FILE [EXTRA_CURL_ARGS...]
http_json() {
  local method="$1" url="$2" data="${3:-}" body_file="$4"
  shift 4
  local args=(--silent --show-error --max-time 15 --request "$method" --output "$body_file" --write-out '%{http_code}')
  while [[ $# -gt 0 ]]; do args+=("$1"); shift; done
  if [[ -n "$data" ]]; then
    args+=(--header 'Content-Type: application/json' --data "$data")
  fi
  curl "${args[@]}" "$url"
}

json_escape() { jq -Rsa .; }

# Fetch a challenge from the API and sign it with the Ed25519 key.
# Sets CHALLENGE and SIGNATURE variables for the caller.
fetch_and_sign_challenge() {
  local challenge_payload
  challenge_payload="$(printf '{"public_key":%s}' "$(printf '%s' "$EDPROOF" | json_escape)")"
  local status
  status="$(http_json POST "$BASE_URL/v1/auth/challenge" "$challenge_payload" "$TMPBODY")"
  if [[ "$status" != "200" ]]; then
    fail "auth/challenge returned HTTP $status: $(cat "$TMPBODY")"
  fi
  CHALLENGE="$(jq -r '.challenge // empty' "$TMPBODY")"
  if [[ -z "$CHALLENGE" ]]; then
    fail "auth/challenge: missing challenge in response"
  fi
  detail "challenge: ${CHALLENGE:0:30}..."

  # Sign the challenge using ssh-keygen SSHSIG format, then base64-encode.
  # The namespace must be "edproof" to match the server's verification.
  local sig_file sig_armored
  sig_file="$(mktemp)"
  printf '%s' "$CHALLENGE" | ssh-keygen -Y sign -f "$KEY_PATH" -n edproof -q > "$sig_file" 2>/dev/null
  # ssh-keygen outputs PEM-armored SSHSIG; extract the binary and base64-encode.
  sig_armored="$(sed '1d;$d' "$sig_file")"
  SIGNATURE="$(printf '%s' "$sig_armored" | tr -d '\n')"
  rm -f "$sig_file"
  detail "signature: ${SIGNATURE:0:30}..."
}

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)       BASE_URL="$2";       shift 2 ;;
    --billing-email)  BILLING_EMAIL="$2";  shift 2 ;;
    --imap-host)      IMAP_HOST="$2";      shift 2 ;;
    --imap-port)      IMAP_PORT="$2";      shift 2 ;;
    --work-dir)       WORK_DIR="$2";       shift 2 ;;
    --key-path)       KEY_PATH="$2";       shift 2 ;;
    --auto-pay)       AUTO_PAY=1;          shift ;;
    --admin-api-key)  ADMIN_API_KEY="$2";  shift 2 ;;
    --discount-code)  DISCOUNT_CODE="$2";  shift 2 ;;
    --verbose)        VERBOSE=1;           shift ;;
    --help|-h)        usage; exit 0 ;;
    *)                echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ "$AUTO_PAY" == "1" && -z "$DISCOUNT_CODE" ]]; then
  echo "auto-pay mode requires --discount-code or SMOKE_DISCOUNT_CODE (a 100%-off Paddle gift coupon — Paddle checkout has no card-free API confirm path)" >&2
  exit 2
fi

require_cmd curl
require_cmd jq
require_cmd ssh-keygen
require_cmd openssl
if [[ "$AUTO_PAY" == "1" ]]; then
  require_cmd node
  if ! node -e "require('playwright')" 2>/dev/null; then
    echo "auto-pay mode requires the 'playwright' npm package (Paddle checkout completion is always browser-driven)" >&2
    exit 2
  fi
fi

# Key management
mkdir -p "$WORK_DIR"
chmod 700 "$WORK_DIR"
if [[ -z "$KEY_PATH" ]]; then
  KEY_PATH="$WORK_DIR/identity"
fi
if [[ "$AUTO_PAY" == "1" ]]; then
  # Fresh key each run — exercises full claim-to-read flow
  rm -f "$KEY_PATH" "$KEY_PATH.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$KEY_PATH" -C "mailservice-smoke-autopay"
  # Unique billing email per run for clean reconciliation/support records.
  RUN_ID="$(date +%s)-$$"
  BILLING_EMAIL="${BILLING_EMAIL%%@*}+${RUN_ID}@${BILLING_EMAIL#*@}"
  detail "billing email (unique): $BILLING_EMAIL"
elif [[ ! -f "$KEY_PATH" || ! -f "$KEY_PATH.pub" ]]; then
  log "Generating Ed25519 key at $KEY_PATH"
  rm -f "$KEY_PATH" "$KEY_PATH.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$KEY_PATH" -C "mailservice-smoke-periodic"
fi
EDPROOF="$(cat "$KEY_PATH.pub")"
detail "fingerprint: $(ssh-keygen -l -E sha256 -f "$KEY_PATH.pub" | awk '{print $2}')"

TMPBODY="$(mktemp)"
trap 'rm -f "$TMPBODY"' EXIT

CHECKS_PASSED=0
if [[ "$AUTO_PAY" == "1" ]]; then
  CHECKS_TOTAL=6
else
  CHECKS_TOTAL=5
fi

CHECK_NUM=0
next_check() { CHECK_NUM=$((CHECK_NUM + 1)); }

# --- Check: Health ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: healthz"
STATUS="$(http_json GET "$BASE_URL/healthz" "" "$TMPBODY")"
if [[ "$STATUS" == "200" ]]; then
  detail "ok"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
  fail "healthz returned HTTP $STATUS"
fi

# --- Check: Claim ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: claim mailbox"
fetch_and_sign_challenge
if [[ "$AUTO_PAY" == "1" && -n "$DISCOUNT_CODE" ]]; then
  detail "applying discount code: $DISCOUNT_CODE"
  CLAIM_PAYLOAD="$(printf '{"billing_email":%s,"edproof":%s,"challenge":%s,"signature":%s,"coupon_code":%s}' \
    "$(printf '%s' "$BILLING_EMAIL" | json_escape)" \
    "$(printf '%s' "$EDPROOF" | json_escape)" \
    "$(printf '%s' "$CHALLENGE" | json_escape)" \
    "$(printf '%s' "$SIGNATURE" | json_escape)" \
    "$(printf '%s' "$DISCOUNT_CODE" | json_escape)")"
else
  CLAIM_PAYLOAD="$(printf '{"billing_email":%s,"edproof":%s,"challenge":%s,"signature":%s}' \
    "$(printf '%s' "$BILLING_EMAIL" | json_escape)" \
    "$(printf '%s' "$EDPROOF" | json_escape)" \
    "$(printf '%s' "$CHALLENGE" | json_escape)" \
    "$(printf '%s' "$SIGNATURE" | json_escape)")"
fi

STATUS="$(http_json POST "$BASE_URL/v1/mailboxes/claim" "$CLAIM_PAYLOAD" "$TMPBODY")"
if [[ "$STATUS" != "200" && "$STATUS" != "201" ]]; then
  fail "claim returned HTTP $STATUS: $(cat "$TMPBODY")"
fi

MAILBOX_STATUS="$(jq -r '.status // empty' "$TMPBODY")"
detail "status: $MAILBOX_STATUS"
CHECKS_PASSED=$((CHECKS_PASSED + 1))

# --- Auto-pay (if enabled) ---
if [[ "$AUTO_PAY" == "1" && "$MAILBOX_STATUS" != "active" ]]; then
  next_check
  log "Check $CHECK_NUM/$CHECKS_TOTAL: auto-pay via Paddle sandbox checkout overlay"

  PAYMENT_URL="$(jq -r '.payment_url // empty' "$TMPBODY")"
  if [[ -z "$PAYMENT_URL" ]]; then
    fail "auto-pay: no payment_url in claim response"
  fi
  detail "checkout page: $PAYMENT_URL"

  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  if ! CHECKOUT_EMAIL="$BILLING_EMAIL" CHECKOUT_VERBOSE="$VERBOSE" \
      node "$SCRIPT_DIR/paddle-checkout-confirm.js" "$PAYMENT_URL"; then
    fail "auto-pay: headless checkout failed"
  fi

  # Poll for mailbox activation (webhook delivery + processing).
  # After initial wait, try reconciliation as fallback for missed webhooks.
  ACTIVATE_TIMEOUT=120
  ACTIVATE_INTERVAL=3
  RECONCILE_AFTER=30
  RECONCILE_INTERVAL=15
  LAST_RECONCILE=0
  ELAPSED=0
  while [[ "$ELAPSED" -lt "$ACTIVATE_TIMEOUT" ]]; do
    # Each re-claim needs a fresh challenge (challenges are single-use)
    fetch_and_sign_challenge
    CLAIM_PAYLOAD="$(printf '{"billing_email":%s,"edproof":%s,"challenge":%s,"signature":%s}' \
      "$(printf '%s' "$BILLING_EMAIL" | json_escape)" \
      "$(printf '%s' "$EDPROOF" | json_escape)" \
      "$(printf '%s' "$CHALLENGE" | json_escape)" \
      "$(printf '%s' "$SIGNATURE" | json_escape)")"
    STATUS="$(http_json POST "$BASE_URL/v1/mailboxes/claim" "$CLAIM_PAYLOAD" "$TMPBODY")"
    MAILBOX_STATUS="$(jq -r '.status // empty' "$TMPBODY")"
    if [[ "$MAILBOX_STATUS" == "active" ]]; then
      break
    fi

    # After RECONCILE_AFTER seconds, trigger reconciliation periodically.
    # Paddle sandbox webhook delivery can be slow — retry every
    # RECONCILE_INTERVAL seconds.
    if [[ "$ELAPSED" -ge "$RECONCILE_AFTER" && -n "$ADMIN_API_KEY" ]]; then
      SINCE_LAST=$((ELAPSED - LAST_RECONCILE))
      if [[ "$LAST_RECONCILE" -eq 0 || "$SINCE_LAST" -ge "$RECONCILE_INTERVAL" ]]; then
        detail "webhook not received after ${ELAPSED}s, triggering reconciliation..."
        RECONCILE_STATUS="$(http_json POST "$BASE_URL/admin/payments/reconcile" "" "$TMPBODY" \
          --header "Authorization: Bearer $ADMIN_API_KEY")"
        RECONCILE_ACTIVATED="$(jq -r '.activated // 0' "$TMPBODY")"
        detail "reconcile: HTTP $RECONCILE_STATUS, activated=$RECONCILE_ACTIVATED"
        # Log per-mailbox results for debugging Paddle sandbox delays
        if [[ "$VERBOSE" == "1" ]]; then
          jq -c '.results[]? // empty' "$TMPBODY" 2>/dev/null | while read -r r; do
            detail "  $(echo "$r" | jq -r '"mbx=\(.mailbox_id) session=\(.session_id) status=\(.status) action=\(.action) error=\(.error // "")"')"
          done
        fi
        LAST_RECONCILE=$ELAPSED
      fi
    fi

    sleep "$ACTIVATE_INTERVAL"
    ELAPSED=$((ELAPSED + ACTIVATE_INTERVAL))
  done

  if [[ "$MAILBOX_STATUS" != "active" ]]; then
    fail "auto-pay: mailbox not active after ${ACTIVATE_TIMEOUT}s (status: $MAILBOX_STATUS)"
  fi

  detail "mailbox activated after ~${ELAPSED}s"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))

elif [[ "$AUTO_PAY" != "1" && "$MAILBOX_STATUS" != "active" ]]; then
  PAYMENT_URL="$(jq -r '.payment_url // empty' "$TMPBODY")"
  echo ""
  echo "Mailbox is not yet active (status: $MAILBOX_STATUS)."
  echo "Complete payment to activate, then re-run:"
  echo "  $PAYMENT_URL"
  echo ""
  echo "After payment, all subsequent runs will pass automatically."
  exit 1

elif [[ "$AUTO_PAY" == "1" && "$MAILBOX_STATUS" == "active" ]]; then
  # Already active (shouldn't happen with fresh keys, but handle gracefully)
  next_check
  log "Check $CHECK_NUM/$CHECKS_TOTAL: auto-pay (skipped — already active)"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
fi

# --- Check: Resolve ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: resolve IMAP credentials"
fetch_and_sign_challenge
RESOLVE_PAYLOAD="$(printf '{"protocol":"imap","edproof":%s,"challenge":%s,"signature":%s}' \
  "$(printf '%s' "$EDPROOF" | json_escape)" \
  "$(printf '%s' "$CHALLENGE" | json_escape)" \
  "$(printf '%s' "$SIGNATURE" | json_escape)")"

STATUS="$(http_json POST "$BASE_URL/v1/access/resolve" "$RESOLVE_PAYLOAD" "$TMPBODY")"
if [[ "$STATUS" != "200" ]]; then
  fail "resolve returned HTTP $STATUS: $(cat "$TMPBODY")"
fi

IMAP_USER="$(jq -r '.username // empty' "$TMPBODY")"
IMAP_PASS="$(jq -r '.password // empty' "$TMPBODY")"
ACCESS_TOKEN="$(jq -r '.access_token // empty' "$TMPBODY")"

if [[ -z "$IMAP_USER" || -z "$IMAP_PASS" || -z "$ACCESS_TOKEN" ]]; then
  fail "resolve missing fields: user=$IMAP_USER pass=${IMAP_PASS:+***} token=${ACCESS_TOKEN:+***}"
fi

detail "user: $IMAP_USER"
CHECKS_PASSED=$((CHECKS_PASSED + 1))

# --- Check: IMAP login ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: IMAP login (TLS, $IMAP_HOST:$IMAP_PORT)"
# Use timeout to prevent hangs; quote credentials for IMAP LOGIN.
TIMEOUT_CMD=""
if command -v timeout >/dev/null 2>&1; then
  TIMEOUT_CMD="timeout 15"
elif command -v gtimeout >/dev/null 2>&1; then
  TIMEOUT_CMD="gtimeout 15"
fi
IMAP_OUTPUT="$(printf 'a001 LOGIN "%s" "%s"\na002 LOGOUT\n' "$IMAP_USER" "$IMAP_PASS" \
  | $TIMEOUT_CMD openssl s_client -connect "$IMAP_HOST:$IMAP_PORT" -quiet 2>/dev/null || true)"

if echo "$IMAP_OUTPUT" | grep -q "a001 OK"; then
  detail "login ok"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
  # Strip LOGIN line to avoid leaking IMAP credentials in CI logs
  fail "IMAP login failed: $(echo "$IMAP_OUTPUT" | grep -v 'LOGIN' | head -5)"
fi

# --- Check: HTTP message API ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: HTTP message API"
MSG_PAYLOAD="$(printf '{"access_token":%s,"unread_only":false,"limit":1,"include_body":false}' \
  "$(printf '%s' "$ACCESS_TOKEN" | json_escape)")"

STATUS="$(http_json POST "$BASE_URL/v1/imap/messages" "$MSG_PAYLOAD" "$TMPBODY")"
if [[ "$STATUS" != "200" ]]; then
  fail "messages API returned HTTP $STATUS: $(cat "$TMPBODY")"
fi

MSG_STATUS="$(jq -r '.status // empty' "$TMPBODY")"
if [[ "$MSG_STATUS" != "ok" ]]; then
  fail "messages API returned status: $MSG_STATUS"
fi

detail "status: ok"
CHECKS_PASSED=$((CHECKS_PASSED + 1))

# --- Summary ---
echo ""
echo "OK: $CHECKS_PASSED/$CHECKS_TOTAL checks passed"
