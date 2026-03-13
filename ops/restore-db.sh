#!/usr/bin/env bash
# Restore SQLite database from S3 backup.
#
# Usage:
#   ops/restore-db.sh                  # Restore latest backup
#   ops/restore-db.sh 2026-03-13       # Restore specific date
#   ops/restore-db.sh --list           # List available backups
#
# Required environment (same as backup-db.sh):
#   BACKUP_S3_BUCKET, BACKUP_S3_ENDPOINT, BACKUP_S3_REGION
#   AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
#
# Optional:
#   DATABASE_PATH       - Path to SQLite DB (default: /var/lib/mailservice/data/mailservice.db)
#   BACKUP_PREFIX       - S3 key prefix (default: mailservice-db)

set -euo pipefail
umask 077

DATABASE_PATH="${DATABASE_PATH:-/var/lib/mailservice/data/mailservice.db}"
PREFIX="${BACKUP_PREFIX:-mailservice-db}"
ACTION="${1:-latest}"

# Secure temp directory
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

log() { echo "[restore] $(date -u '+%Y-%m-%d %H:%M:%S') $*"; }
fail() { log "FAIL: $*" >&2; exit 1; }

# Validate required env
for var in BACKUP_S3_BUCKET BACKUP_S3_ENDPOINT BACKUP_S3_REGION AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY; do
  if [ -z "${!var:-}" ]; then
    fail "missing required env: $var"
  fi
done

list_backups() {
  aws s3api list-objects-v2 \
    --bucket "$BACKUP_S3_BUCKET" \
    --prefix "$PREFIX/" \
    --endpoint-url "$BACKUP_S3_ENDPOINT" \
    --region "$BACKUP_S3_REGION" \
    --query "Contents[].{Key:Key,Size:Size,Modified:LastModified}" \
    --output table 2>/dev/null
}

if [ "$ACTION" = "--list" ]; then
  log "available backups:"
  list_backups
  exit 0
fi

# Find the backup to restore
if [ "$ACTION" = "latest" ]; then
  S3_KEY="$(aws s3api list-objects-v2 \
    --bucket "$BACKUP_S3_BUCKET" \
    --prefix "$PREFIX/" \
    --endpoint-url "$BACKUP_S3_ENDPOINT" \
    --region "$BACKUP_S3_REGION" \
    --query "sort_by(Contents, &LastModified)[-1].Key" \
    --output text 2>/dev/null)"
  if [ -z "$S3_KEY" ] || [ "$S3_KEY" = "None" ]; then
    fail "no backups found in s3://${BACKUP_S3_BUCKET}/${PREFIX}/"
  fi
else
  # Find backup matching the given date
  S3_KEY="$(aws s3api list-objects-v2 \
    --bucket "$BACKUP_S3_BUCKET" \
    --prefix "$PREFIX/${ACTION}" \
    --endpoint-url "$BACKUP_S3_ENDPOINT" \
    --region "$BACKUP_S3_REGION" \
    --query "sort_by(Contents, &LastModified)[-1].Key" \
    --output text 2>/dev/null)"
  if [ -z "$S3_KEY" ] || [ "$S3_KEY" = "None" ]; then
    fail "no backup found matching date: $ACTION"
  fi
fi

log "restoring from: s3://${BACKUP_S3_BUCKET}/${S3_KEY}"

# Download
DOWNLOAD_FILE="${TMPDIR}/restore.db.gz"
aws s3 cp "s3://${BACKUP_S3_BUCKET}/${S3_KEY}" "$DOWNLOAD_FILE" \
  --endpoint-url "$BACKUP_S3_ENDPOINT" \
  --region "$BACKUP_S3_REGION" \
  --no-progress \
  --quiet
log "downloaded: $(du -h "$DOWNLOAD_FILE" | cut -f1)"

# Decompress
RESTORED_FILE="${TMPDIR}/restore.db"
gunzip "$DOWNLOAD_FILE"
log "decompressed: $(du -h "$RESTORED_FILE" | cut -f1)"

# Validate the restored file is valid SQLite
if ! sqlite3 "$RESTORED_FILE" "PRAGMA integrity_check;" | head -1 | grep -q "ok"; then
  fail "downloaded backup is not a valid SQLite database"
fi
log "integrity check: ok"

ROW_COUNT="$(sqlite3 "$RESTORED_FILE" "SELECT COUNT(*) FROM mailboxes;" 2>/dev/null || echo "?")"
log "mailboxes in backup: $ROW_COUNT"

# Back up current database before overwriting
if [ -f "$DATABASE_PATH" ]; then
  SAFETY_BACKUP="${DATABASE_PATH}.pre-restore.$(date -u '+%s')"
  cp "$DATABASE_PATH" "$SAFETY_BACKUP"
  log "current database backed up to: $SAFETY_BACKUP"
fi

# Stop services that use the database
log "IMPORTANT: stop mailservice-api, postfix, and dovecot2 before replacing the database."
log "  systemctl stop mailservice-api postfix dovecot2"
log ""
log "restored database ready at: $RESTORED_FILE"
log "to apply: cp '$RESTORED_FILE' '$DATABASE_PATH' && chown mailservice:mailservice '$DATABASE_PATH'"
log "then restart: systemctl start mailservice-api postfix dovecot2"
log ""
log "NOTE: temp dir $TMPDIR will be removed on exit. Copy the file before this script ends."
