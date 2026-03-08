#!/usr/bin/env bash

set -euo pipefail

BASE_URL="https://truevipaccess.com"
BILLING_EMAIL=""
WORK_DIR="${TMPDIR:-/tmp}/mailservice-smoke"
KEY_PATH=""
EDPROOF_VALUE=""
EDPROOF_FILE=""
RESOLVE_TIMEOUT_SECONDS=600
POLL_INTERVAL_SECONDS=10

usage() {
  cat <<'EOF'
Usage:
  ops/smoke-test-mailbox.sh --billing-email you@example.com [options]

Options:
  --base-url URL                Base URL to test. Default: https://truevipaccess.com
  --billing-email EMAIL         Billing email for mailbox claim. Required.
  --work-dir DIR                Working directory for generated key material.
  --key-path PATH               Key path. Default: <work-dir>/identity
  --edproof VALUE               Explicit edproof payload to send.
  --edproof-file PATH           File whose contents should be sent as edproof.
  --resolve-timeout SECONDS     How long to wait for payment activation. Default: 600
  --poll-interval SECONDS       Resolve polling interval. Default: 10
  --help                        Show this help.

Notes:
  - This script generates an Ed25519 key pair if one does not already exist.
  - By default it sends the contents of <key-path>.pub as the edproof payload.
    Override that with --edproof or --edproof-file if your verifier expects a
    different proof blob.
  - Payment is still manual. The script prints the payment URL and then polls
    /v1/access/resolve until the mailbox becomes active.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

json_escape() {
  jq -Rsa .
}

write_json_request() {
  local billing_email="$1"
  local edproof="$2"
  printf '{"billing_email":%s,"edproof":%s}' \
    "$(printf '%s' "$billing_email" | json_escape)" \
    "$(printf '%s' "$edproof" | json_escape)"
}

write_resolve_request() {
  local edproof="$1"
  printf '{"protocol":"imap","edproof":%s}' \
    "$(printf '%s' "$edproof" | json_escape)"
}

http_json() {
  local method="$1"
  local url="$2"
  local data="${3:-}"
  local body_file="$4"
  local status

  if [[ -n "$data" ]]; then
    status="$(curl --silent --show-error \
      --request "$method" \
      --header 'Content-Type: application/json' \
      --data "$data" \
      --output "$body_file" \
      --write-out '%{http_code}' \
      "$url")"
  else
    status="$(curl --silent --show-error \
      --request "$method" \
      --output "$body_file" \
      --write-out '%{http_code}' \
      "$url")"
  fi

  printf '%s' "$status"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="$2"
      shift 2
      ;;
    --billing-email)
      BILLING_EMAIL="$2"
      shift 2
      ;;
    --work-dir)
      WORK_DIR="$2"
      shift 2
      ;;
    --key-path)
      KEY_PATH="$2"
      shift 2
      ;;
    --edproof)
      EDPROOF_VALUE="$2"
      shift 2
      ;;
    --edproof-file)
      EDPROOF_FILE="$2"
      shift 2
      ;;
    --resolve-timeout)
      RESOLVE_TIMEOUT_SECONDS="$2"
      shift 2
      ;;
    --poll-interval)
      POLL_INTERVAL_SECONDS="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$BILLING_EMAIL" ]]; then
  echo "--billing-email is required" >&2
  usage >&2
  exit 1
fi

require_cmd curl
require_cmd jq
require_cmd ssh-keygen

mkdir -p "$WORK_DIR"
if [[ -z "$KEY_PATH" ]]; then
  KEY_PATH="$WORK_DIR/identity"
fi

if [[ ! -f "$KEY_PATH" || ! -f "$KEY_PATH.pub" ]]; then
  rm -f "$KEY_PATH" "$KEY_PATH.pub"
  ssh-keygen -q -t ed25519 -N "" -f "$KEY_PATH" -C "mailservice-smoke@$(hostname -s 2>/dev/null || echo local)"
fi

FINGERPRINT="$(ssh-keygen -l -E sha256 -f "$KEY_PATH.pub" | awk '{print $2}')"

if [[ -n "$EDPROOF_VALUE" && -n "$EDPROOF_FILE" ]]; then
  echo "use either --edproof or --edproof-file, not both" >&2
  exit 1
fi

if [[ -n "$EDPROOF_FILE" ]]; then
  EDPROOF_VALUE="$(cat "$EDPROOF_FILE")"
elif [[ -z "$EDPROOF_VALUE" ]]; then
  EDPROOF_VALUE="$(cat "$KEY_PATH.pub")"
fi

HEALTH_BODY="$(mktemp)"
CLAIM_BODY="$(mktemp)"
RESOLVE_BODY="$(mktemp)"
trap 'rm -f "$HEALTH_BODY" "$CLAIM_BODY" "$RESOLVE_BODY"' EXIT

echo "==> Checking health at $BASE_URL/healthz"
HEALTH_STATUS="$(http_json GET "$BASE_URL/healthz" "" "$HEALTH_BODY")"
if [[ "$HEALTH_STATUS" != "200" ]]; then
  echo "health check failed: HTTP $HEALTH_STATUS" >&2
  cat "$HEALTH_BODY" >&2
  exit 1
fi
cat "$HEALTH_BODY"
echo

echo "==> Using key path: $KEY_PATH"
echo "==> Public fingerprint: $FINGERPRINT"

echo "==> Claiming mailbox for $BILLING_EMAIL"
CLAIM_STATUS="$(http_json POST "$BASE_URL/v1/mailboxes/claim" "$(write_json_request "$BILLING_EMAIL" "$EDPROOF_VALUE")" "$CLAIM_BODY")"

if [[ "$CLAIM_STATUS" != "200" && "$CLAIM_STATUS" != "201" ]]; then
  echo "mailbox claim failed: HTTP $CLAIM_STATUS" >&2
  cat "$CLAIM_BODY" >&2
  exit 1
fi

MAILBOX_ID="$(jq -r '.id // empty' "$CLAIM_BODY")"
MAILBOX_EMAIL="$(jq -r '.email // empty' "$CLAIM_BODY")"
PAYMENT_URL="$(jq -r '.payment_url // empty' "$CLAIM_BODY")"
MAILBOX_STATUS="$(jq -r '.status // empty' "$CLAIM_BODY")"

echo "==> Mailbox claimed"
echo "    mailbox_id: ${MAILBOX_ID:-<unknown>}"
echo "    mailbox_email: ${MAILBOX_EMAIL:-<unknown>}"
echo "    status: ${MAILBOX_STATUS:-<unknown>}"

if [[ -n "$PAYMENT_URL" && "$PAYMENT_URL" != "null" ]]; then
  echo "    payment_url: $PAYMENT_URL"
fi

if [[ "$MAILBOX_STATUS" != "active" ]]; then
  echo
  echo "==> Complete payment in your browser:"
  if [[ -n "$PAYMENT_URL" && "$PAYMENT_URL" != "null" ]]; then
    echo "    $PAYMENT_URL"
  else
    echo "    no payment_url returned" >&2
    exit 1
  fi
  echo
  echo "==> Polling /v1/access/resolve until the mailbox becomes active"
fi

DEADLINE=$((SECONDS + RESOLVE_TIMEOUT_SECONDS))
while true; do
  RESOLVE_STATUS="$(http_json POST "$BASE_URL/v1/access/resolve" "$(write_resolve_request "$EDPROOF_VALUE")" "$RESOLVE_BODY")"

  case "$RESOLVE_STATUS" in
    200)
      echo "==> Resolve succeeded"
      jq . "$RESOLVE_BODY"
      break
      ;;
    409)
      CURRENT_STATUS="$(jq -r '.status // empty' "$RESOLVE_BODY")"
      if [[ "$CURRENT_STATUS" != "waiting_payment" ]]; then
        echo "unexpected conflict response:" >&2
        cat "$RESOLVE_BODY" >&2
        exit 1
      fi
      if (( SECONDS >= DEADLINE )); then
        echo "timed out waiting for payment activation" >&2
        cat "$RESOLVE_BODY" >&2
        exit 1
      fi
      echo "    still waiting for payment; retrying in ${POLL_INTERVAL_SECONDS}s"
      sleep "$POLL_INTERVAL_SECONDS"
      ;;
    *)
      echo "resolve failed: HTTP $RESOLVE_STATUS" >&2
      cat "$RESOLVE_BODY" >&2
      exit 1
      ;;
  esac
done
