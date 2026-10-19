#!/usr/bin/env bash
#
# Periodic smoke test for mailservice.
# Designed to run every 5 minutes via cron or GitHub Actions schedule.
#
# Checkout-only mode creates a fresh unpaid mailbox and verifies the checkout.
# Persistent mode can also exercise read paths after an operator has paid it.
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
CHECKOUT_ONLY="${SMOKE_CHECKOUT_ONLY:-0}"

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
  --checkout-only         Verify an unpaid OC checkout. Env: SMOKE_CHECKOUT_ONLY=1
  --verbose               Print details.          Env: SMOKE_VERBOSE=1
  --help                  Show this help.

Persistent mode reuses a key after manual payment and checks IMAP access.
Checkout-only mode creates a fresh key, checks the OC checkout URL and pending
status, then prints SKIP for activation-dependent checks (no payment is made).

Checks performed:
  1. GET  /healthz                — API is up
  2. POST /v1/mailboxes/claim     — claim succeeds
  3. Checkout URL and state       — OC tier checkout with tag, pending_payment
  4. POST /v1/access/resolve      — active mailbox returns IMAP credentials
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
    --checkout-only) CHECKOUT_ONLY=1; shift ;;
    --verbose)        VERBOSE=1;           shift ;;
    --help|-h)        usage; exit 0 ;;
    *)                echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

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
if [[ "$CHECKOUT_ONLY" == "1" ]]; then
  # Fresh key each run keeps the checkout test independent of prior payments
  rm -f "$KEY_PATH" "$KEY_PATH.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$KEY_PATH" -C "mailservice-smoke-checkout"
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
CHECKS_TOTAL=6

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

# --- Check: Open Collective checkout ---
next_check
log "Check $CHECK_NUM/$CHECKS_TOTAL: Open Collective checkout and pending state"
PAYMENT_URL="$(jq -r '.payment_url // empty' "$TMPBODY")"
if [[ "$MAILBOX_STATUS" == "pending_payment" ]]; then
  [[ "$PAYMENT_URL" =~ ^https://opencollective[.]com/.*/contribute/mailbox-monthly-[0-9]+/checkout[?].*tags=tva-[a-z2-7]{16} ]] || \
    fail "claim returned unexpected Open Collective payment_url: $PAYMENT_URL"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
  echo "SKIP: activation, resolve, IMAP login and message read require a real payment; no operator-forced activation path exists on main"
  echo "OK: $CHECKS_PASSED checks passed; 3 activation-dependent checks skipped"
  exit 0
fi
if [[ "$CHECKOUT_ONLY" == "1" ]]; then
  fail "checkout-only claim status is $MAILBOX_STATUS, want pending_payment"
fi
if [[ "$MAILBOX_STATUS" != "active" ]]; then
  fail "claim status is $MAILBOX_STATUS, want pending_payment or active"
fi
echo "SKIP: checkout shape check on already active persistent mailbox"

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
