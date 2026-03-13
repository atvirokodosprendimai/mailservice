#!/usr/bin/env bash
#
# Periodic smoke test for mailservice.
# Designed to run every 5 minutes via cron or GitHub Actions schedule.
#
# Two modes:
#   1. Persistent key mode (default): reuses a key after first manual payment.
#   2. Auto-pay mode (--auto-pay): generates a fresh key each run and
#      auto-confirms via Polar sandbox API. Requires a free sandbox product.
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
POLAR_TOKEN="${SMOKE_POLAR_TOKEN:-}"
POLAR_API="${SMOKE_POLAR_API:-https://sandbox-api.polar.sh}"

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
  --polar-token TOKEN     Polar API token.        Env: SMOKE_POLAR_TOKEN
  --polar-api URL         Polar API base URL.     Env: SMOKE_POLAR_API
  --verbose               Print details.          Env: SMOKE_VERBOSE=1
  --help                  Show this help.

Persistent key mode (default):
  Generates an Ed25519 key and claims a mailbox.
  If the mailbox is not yet paid, prints the payment URL and exits 1.
  After manual payment, all subsequent runs pass automatically.

Auto-pay mode (--auto-pay):
  Generates a fresh key each run and auto-confirms the checkout via the
  Polar API. Requires the smoke server to use a free sandbox product.
  Each run exercises the full claim → pay → activate → resolve → read flow.

Checks performed:
  1. GET  /healthz                — API is up
  2. POST /v1/mailboxes/claim     — claim succeeds
  3. Auto-pay (if enabled)        — confirm checkout via Polar sandbox
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

http_json_polar() {
  local method="$1" path="$2" data="${3:-}" body_file="$4"
  http_json "$method" "${POLAR_API}${path}" "$data" "$body_file" \
    --location --header "Authorization: Bearer $POLAR_TOKEN"
}

http_json_polar_client() {
  local method="$1" path="$2" data="${3:-}" body_file="$4"
  http_json "$method" "${POLAR_API}${path}" "$data" "$body_file" --location
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
    --polar-token)    POLAR_TOKEN="$2";    shift 2 ;;
    --polar-api)      POLAR_API="$2";      shift 2 ;;
    --verbose)        VERBOSE=1;           shift ;;
    --help|-h)        usage; exit 0 ;;
    *)                echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ "$AUTO_PAY" == "1" && -z "$POLAR_TOKEN" ]]; then
  echo "auto-pay mode requires --polar-token or SMOKE_POLAR_TOKEN" >&2
  exit 2
fi

require_cmd curl
require_cmd jq
require_cmd ssh-keygen
require_cmd openssl

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
CLAIM_PAYLOAD="$(printf '{"billing_email":%s,"edproof":%s,"challenge":%s,"signature":%s}' \
  "$(printf '%s' "$BILLING_EMAIL" | json_escape)" \
  "$(printf '%s' "$EDPROOF" | json_escape)" \
  "$(printf '%s' "$CHALLENGE" | json_escape)" \
  "$(printf '%s' "$SIGNATURE" | json_escape)")"

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
  log "Check $CHECK_NUM/$CHECKS_TOTAL: auto-pay via Polar sandbox"

  PAYMENT_URL="$(jq -r '.payment_url // empty' "$TMPBODY")"
  if [[ -z "$PAYMENT_URL" ]]; then
    fail "auto-pay: no payment_url in claim response"
  fi

  # Extract client_secret from payment URL (last path segment)
  CLIENT_SECRET="${PAYMENT_URL##*/}"
  if [[ -z "$CLIENT_SECRET" || "$CLIENT_SECRET" != polar_c_* ]]; then
    fail "auto-pay: cannot extract client_secret from payment_url: $PAYMENT_URL"
  fi
  detail "client_secret: ${CLIENT_SECRET:0:20}..."

  # Confirm checkout via headless browser (Polar requires Stripe Elements).
  # Falls back to legacy API-based confirm if Playwright is not installed.
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  if command -v node >/dev/null 2>&1 && node -e "require('playwright')" 2>/dev/null; then
    detail "confirming via headless browser..."
    CHECKOUT_EMAIL="$BILLING_EMAIL" CHECKOUT_VERBOSE="$VERBOSE" \
      node "$SCRIPT_DIR/polar-checkout-confirm.js" "$PAYMENT_URL" || \
      fail "auto-pay: headless checkout failed"
    CONFIRM_STATUS="confirmed"
  else
    detail "playwright not available, trying API-based confirm..."
    STATUS="$(http_json_polar_client POST "/v1/checkouts/client/$CLIENT_SECRET/confirm" '{}' "$TMPBODY")"
    if [[ "$STATUS" != "200" ]]; then
      fail "auto-pay: confirm returned HTTP $STATUS (install playwright for headless checkout): $(cat "$TMPBODY")"
    fi
    CONFIRM_STATUS="$(jq -r '.status // empty' "$TMPBODY")"
  fi
  detail "confirm status: $CONFIRM_STATUS"

  # Poll for mailbox activation (webhook delivery + processing)
  # Polar sandbox can take several minutes to deliver webhooks
  ACTIVATE_TIMEOUT=90
  ACTIVATE_INTERVAL=3
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
