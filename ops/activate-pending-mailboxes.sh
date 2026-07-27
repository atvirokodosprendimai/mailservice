#!/usr/bin/env bash
# Operator-forced activation of pending_payment mailboxes.
#
# WHY THIS EXISTS
#   Polar terminated the merchant account for this business, so the normal
#   payment-verification path (POST /admin/payments/reconcile) can no longer
#   confirm sessions with the provider — every mailbox comes back "skipped".
#   This script activates pending mailboxes on operator authority instead,
#   WITHOUT payment verification. Use it only when you have independently
#   decided these customers should have service.
#
# WHAT "ACTIVATION" ACTUALLY REQUIRES
#   Marking a mailbox active is TWO writes to TWO different databases:
#     1. Turso (app DB)   — mailboxes.status/paid_at/expires_at
#     2. Mail host SQLite — the Postfix/Dovecot virtual mailbox row
#   The running service does both inside MarkMailboxPaid (which calls
#   provisioner.EnsureMailbox). A bare SQL UPDATE does only #1, which leaves
#   customers looking "active" while receiving no mail. This script therefore
#   updates Turso and then calls POST /admin/mailboxes/reprovision for each
#   mailbox so provisioning actually happens.
#
# SAFETY
#   Dry run by default. Prints exactly what it would change and exits.
#   Pass --apply to execute. Every applied row is logged to a timestamped file
#   so the change set is auditable and reversible.
#
# USAGE
#   export TURSO_DATABASE_URL=...   TURSO_AUTH_TOKEN=...
#   export ADMIN_API_KEY=...        PUBLIC_BASE_URL=https://truevipaccess.com
#   ./ops/activate-pending-mailboxes.sh            # dry run
#   ./ops/activate-pending-mailboxes.sh --apply    # execute
#
#   Optional: --months N   grant N months instead of 1 (default 1)
#             --limit N    cap how many mailboxes are touched (default: no cap)

set -euo pipefail

APPLY=0
MONTHS=1
LIMIT=0

while [ $# -gt 0 ]; do
  case "$1" in
    --apply)  APPLY=1; shift ;;
    --months) MONTHS="${2:?--months needs a value}"; shift 2 ;;
    --limit)  LIMIT="${2:?--limit needs a value}"; shift 2 ;;
    -h|--help) sed -n '2,32p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    echo "missing required env var: $name" >&2
    exit 2
  fi
}
require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 2; }
}

require_cmd turso
require_cmd jq
require_cmd curl
require_env TURSO_DATABASE_URL
require_env TURSO_AUTH_TOKEN
require_env ADMIN_API_KEY
require_env PUBLIC_BASE_URL

BASE_URL="${PUBLIC_BASE_URL%/}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
LOGFILE="activate-pending-mailboxes-${STAMP}.log"

# Portable UTC "now + N months". GNU date and BSD/macOS date differ.
if date -u -d "+${MONTHS} months" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
  NEW_EXPIRY="$(date -u -d "+${MONTHS} months" +%Y-%m-%dT%H:%M:%SZ)"
else
  NEW_EXPIRY="$(date -u -v "+${MONTHS}m" +%Y-%m-%dT%H:%M:%SZ)"
fi
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

turso_query() {
  turso db shell "$TURSO_DATABASE_URL" "$1"
}

SELECT_SQL="SELECT id, address, owner_email, public_key, payment_session_id, granted_months, coupon_used, created_at
FROM mailboxes
WHERE status = 'pending_payment'
ORDER BY created_at ASC;"

echo "== pending_payment mailboxes =="
echo "Turso:        ${TURSO_DATABASE_URL}"
echo "Service:      ${BASE_URL}"
echo "Grant:        ${MONTHS} month(s) -> expires_at ${NEW_EXPIRY}"
echo "Mode:         $([ "$APPLY" -eq 1 ] && echo 'APPLY (writes)' || echo 'DRY RUN (no writes)')"
echo

ROWS_JSON="$(turso db shell "$TURSO_DATABASE_URL" ".mode json
${SELECT_SQL}" 2>/dev/null | jq -c '.' 2>/dev/null || echo '[]')"

COUNT="$(echo "$ROWS_JSON" | jq 'length')"
if [ "$COUNT" = "0" ] || [ -z "$COUNT" ]; then
  echo "No mailboxes in pending_payment. Nothing to do."
  exit 0
fi

if [ "$LIMIT" -gt 0 ]; then
  ROWS_JSON="$(echo "$ROWS_JSON" | jq -c ".[0:${LIMIT}]")"
  echo "Limiting to first ${LIMIT} of ${COUNT}."
  COUNT="$(echo "$ROWS_JSON" | jq 'length')"
fi

echo "$ROWS_JSON" | jq -r '.[] | "  \(.id)  \(.address // "-")  \(.owner_email // "-")  created=\(.created_at // "-")  granted_months=\(.granted_months // 0)"'
echo
echo "Total to activate: ${COUNT}"
echo

if [ "$APPLY" -ne 1 ]; then
  cat <<'EOF'
DRY RUN — nothing was changed.

Review the list above carefully. These mailboxes will be granted paid service
WITHOUT any payment having been verified with a provider.

Re-run with --apply to execute.
EOF
  exit 0
fi

echo "Applying. Audit log: ${LOGFILE}"
{
  echo "# activate-pending-mailboxes ${STAMP}"
  echo "# months=${MONTHS} new_expiry=${NEW_EXPIRY} count=${COUNT}"
  echo "# NOTE: activation performed on operator authority, no payment verification"
} >> "$LOGFILE"

ok=0
failed=0

while IFS= read -r row; do
  id="$(echo "$row" | jq -r '.id')"
  address="$(echo "$row" | jq -r '.address // ""')"
  owner_email="$(echo "$row" | jq -r '.owner_email // ""')"
  public_key="$(echo "$row" | jq -r '.public_key // ""')"

  # Honor an existing gift-coupon grant rather than flattening it to --months.
  granted="$(echo "$row" | jq -r '.granted_months // 0')"
  row_months="$MONTHS"
  if [ "$granted" -gt 0 ] 2>/dev/null; then
    row_months="$granted"
  fi
  if date -u -d "+${row_months} months" +%Y-%m-%dT%H:%M:%SZ >/dev/null 2>&1; then
    row_expiry="$(date -u -d "+${row_months} months" +%Y-%m-%dT%H:%M:%SZ)"
  else
    row_expiry="$(date -u -v "+${row_months}m" +%Y-%m-%dT%H:%M:%SZ)"
  fi

  # 1) App DB: flip status and set paid_at/expires_at.
  if ! turso_query "UPDATE mailboxes
       SET status = 'active', paid_at = '${NOW}', expires_at = '${row_expiry}'
       WHERE id = '${id}' AND status = 'pending_payment';" >/dev/null 2>&1; then
    echo "  FAIL(db)     ${id} ${address}"
    echo "FAIL db ${id} ${address}" >> "$LOGFILE"
    failed=$((failed+1))
    continue
  fi

  # 2) Mail host: provision the Postfix/Dovecot virtual mailbox. Without this
  #    the row says active but no mail is ever delivered.
  if [ -z "$owner_email" ] || [ -z "$public_key" ]; then
    echo "  WARN(prov)   ${id} ${address} — missing owner_email/public_key, DB updated but NOT provisioned"
    echo "WARN not-provisioned ${id} ${address} missing-fields" >> "$LOGFILE"
    failed=$((failed+1))
    continue
  fi

  http_code="$(curl -sS -o /tmp/reprov-out.$$ -w '%{http_code}' \
    -X POST "${BASE_URL}/admin/mailboxes/reprovision" \
    -H "Authorization: Bearer ${ADMIN_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg id "$id" --arg oe "$owner_email" --arg pk "$public_key" --arg ex "$row_expiry" \
          '{mailbox_id:$id, owner_email:$oe, public_key:$pk, expires_at:$ex}')" || echo 000)"

  if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
    echo "  OK           ${id} ${address}  expires=${row_expiry}"
    echo "OK ${id} ${address} expires=${row_expiry} months=${row_months}" >> "$LOGFILE"
    ok=$((ok+1))
  else
    echo "  WARN(prov)   ${id} ${address} — DB updated, reprovision HTTP ${http_code}"
    echo "WARN reprovision-failed ${id} ${address} http=${http_code} $(cat /tmp/reprov-out.$$ 2>/dev/null | head -c 200)" >> "$LOGFILE"
    failed=$((failed+1))
  fi
  rm -f /tmp/reprov-out.$$
done < <(echo "$ROWS_JSON" | jq -c '.[]')

echo
echo "Activated+provisioned: ${ok}"
echo "Needs attention:       ${failed}"
echo "Audit log:             ${LOGFILE}"

if [ "$failed" -gt 0 ]; then
  cat <<EOF

Some rows did not fully provision. Those mailboxes are marked active in the app
DB but may not receive mail. Check ${LOGFILE}, then re-run provisioning for the
affected IDs via POST ${BASE_URL}/admin/mailboxes/reprovision.
EOF
  exit 1
fi
