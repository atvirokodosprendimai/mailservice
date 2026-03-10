#!/usr/bin/env bash
#
# Periodic smoke test for mailservice.
# Designed to run every 5 minutes via cron or GitHub Actions schedule.
#
# Uses a persistent Ed25519 key so that after the first manual payment,
# subsequent runs reuse the active mailbox without needing to pay again.
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
  --verbose               Print details.          Env: SMOKE_VERBOSE=1
  --help                  Show this help.

First run:
  The script generates an Ed25519 key and claims a mailbox.
  If the mailbox is not yet paid, it prints the payment URL and exits 1.
  After manual payment, all subsequent runs pass automatically.

Checks performed:
  1. GET  /healthz                — API is up
  2. POST /v1/mailboxes/claim     — claim succeeds (idempotent for active keys)
  3. POST /v1/access/resolve      — returns IMAP credentials
  4. IMAP LOGIN via TLS           — Dovecot authenticates
  5. POST /v1/imap/messages       — HTTP API returns messages
EOF
}

log() { echo "==> $*"; }
detail() { [[ "$VERBOSE" == "1" ]] && echo "    $*" || true; }
fail() { echo "FAIL: $*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 2; }
}

http_json() {
  local method="$1" url="$2" data="${3:-}" body_file="$4"
  if [[ -n "$data" ]]; then
    curl --silent --show-error --max-time 15 \
      --request "$method" \
      --header 'Content-Type: application/json' \
      --data "$data" \
      --output "$body_file" \
      --write-out '%{http_code}' \
      "$url"
  else
    curl --silent --show-error --max-time 15 \
      --request "$method" \
      --output "$body_file" \
      --write-out '%{http_code}' \
      "$url"
  fi
}

json_escape() { jq -Rsa .; }

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)       BASE_URL="$2";       shift 2 ;;
    --billing-email)  BILLING_EMAIL="$2";  shift 2 ;;
    --imap-host)      IMAP_HOST="$2";      shift 2 ;;
    --imap-port)      IMAP_PORT="$2";      shift 2 ;;
    --work-dir)       WORK_DIR="$2";       shift 2 ;;
    --key-path)       KEY_PATH="$2";       shift 2 ;;
    --verbose)        VERBOSE=1;           shift ;;
    --help|-h)        usage; exit 0 ;;
    *)                echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

require_cmd curl
require_cmd jq
require_cmd ssh-keygen
require_cmd openssl

# Ensure persistent key
mkdir -p "$WORK_DIR"
chmod 700 "$WORK_DIR"
if [[ -z "$KEY_PATH" ]]; then
  KEY_PATH="$WORK_DIR/identity"
fi
if [[ ! -f "$KEY_PATH" || ! -f "$KEY_PATH.pub" ]]; then
  log "Generating Ed25519 key at $KEY_PATH"
  rm -f "$KEY_PATH" "$KEY_PATH.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$KEY_PATH" -C "mailservice-smoke-periodic"
fi
EDPROOF="$(cat "$KEY_PATH.pub")"
detail "fingerprint: $(ssh-keygen -l -E sha256 -f "$KEY_PATH.pub" | awk '{print $2}')"

TMPBODY="$(mktemp)"
trap 'rm -f "$TMPBODY"' EXIT

CHECKS_PASSED=0
CHECKS_TOTAL=5

# --- Check 1: Health ---
log "Check 1/5: healthz"
STATUS="$(http_json GET "$BASE_URL/healthz" "" "$TMPBODY")"
if [[ "$STATUS" == "200" ]]; then
  detail "ok"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
  fail "healthz returned HTTP $STATUS"
fi

# --- Check 2: Claim ---
log "Check 2/5: claim mailbox"
CLAIM_PAYLOAD="$(printf '{"billing_email":%s,"edproof":%s}' \
  "$(printf '%s' "$BILLING_EMAIL" | json_escape)" \
  "$(printf '%s' "$EDPROOF" | json_escape)")"

STATUS="$(http_json POST "$BASE_URL/v1/mailboxes/claim" "$CLAIM_PAYLOAD" "$TMPBODY")"
if [[ "$STATUS" != "200" && "$STATUS" != "201" ]]; then
  fail "claim returned HTTP $STATUS: $(cat "$TMPBODY")"
fi

MAILBOX_STATUS="$(jq -r '.status // empty' "$TMPBODY")"
detail "status: $MAILBOX_STATUS"
CHECKS_PASSED=$((CHECKS_PASSED + 1))

if [[ "$MAILBOX_STATUS" != "active" ]]; then
  PAYMENT_URL="$(jq -r '.payment_url // empty' "$TMPBODY")"
  echo ""
  echo "Mailbox is not yet active (status: $MAILBOX_STATUS)."
  echo "Complete payment to activate, then re-run:"
  echo "  $PAYMENT_URL"
  echo ""
  echo "After payment, all subsequent runs will pass automatically."
  exit 1
fi

# --- Check 3: Resolve ---
log "Check 3/5: resolve IMAP credentials"
RESOLVE_PAYLOAD="$(printf '{"protocol":"imap","edproof":%s}' \
  "$(printf '%s' "$EDPROOF" | json_escape)")"

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

# --- Check 4: IMAP login ---
log "Check 4/5: IMAP login (TLS, $IMAP_HOST:$IMAP_PORT)"
IMAP_OUTPUT="$(printf 'a001 LOGIN %s %s\na002 LOGOUT\n' "$IMAP_USER" "$IMAP_PASS" \
  | openssl s_client -connect "$IMAP_HOST:$IMAP_PORT" -quiet 2>/dev/null || true)"

if echo "$IMAP_OUTPUT" | grep -q "a001 OK"; then
  detail "login ok"
  CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
  fail "IMAP login failed: $(echo "$IMAP_OUTPUT" | head -5)"
fi

# --- Check 5: HTTP message API ---
log "Check 5/5: HTTP message API"
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
