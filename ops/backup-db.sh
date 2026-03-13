#!/usr/bin/env bash
# Daily SQLite backup to Hetzner S3 Object Storage.
#
# Uses sqlite3 .backup (online backup API) — safe for concurrent reads/writes.
# Compresses with gzip, uploads with date-stamped key, prunes old backups.
#
# Required environment:
#   BACKUP_S3_BUCKET    - S3 bucket name
#   BACKUP_S3_ENDPOINT  - S3 endpoint URL (e.g. https://fsn1.your-objectstorage.com)
#   BACKUP_S3_REGION    - S3 region (e.g. eu-central-1)
#   AWS_ACCESS_KEY_ID   - S3 access key
#   AWS_SECRET_ACCESS_KEY - S3 secret key
#
# Optional:
#   DATABASE_PATH       - Path to SQLite DB (default: /var/lib/mailservice/data/mailservice.db)
#   BACKUP_RETENTION_DAYS - Days to keep backups (default: 30)
#   BACKUP_PREFIX       - S3 key prefix (default: mailservice-db)

set -euo pipefail

DATABASE_PATH="${DATABASE_PATH:-/var/lib/mailservice/data/mailservice.db}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
PREFIX="${BACKUP_PREFIX:-mailservice-db}"
TIMESTAMP="$(date -u '+%Y-%m-%dT%H%M%SZ')"
BACKUP_FILE="/tmp/mailservice-backup-${TIMESTAMP}.db"
COMPRESSED_FILE="${BACKUP_FILE}.gz"
S3_KEY="${PREFIX}/${TIMESTAMP}.db.gz"

log() { echo "[backup] $(date -u '+%Y-%m-%d %H:%M:%S') $*"; }
fail() { log "FAIL: $*" >&2; exit 1; }

# Validate required env
for var in BACKUP_S3_BUCKET BACKUP_S3_ENDPOINT BACKUP_S3_REGION AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY; do
  if [ -z "${!var:-}" ]; then
    fail "missing required env: $var"
  fi
done

# Validate source database exists
if [ ! -f "$DATABASE_PATH" ]; then
  fail "database not found: $DATABASE_PATH"
fi

# 1. Online backup (safe for concurrent access)
log "backing up $DATABASE_PATH"
sqlite3 "$DATABASE_PATH" ".backup '$BACKUP_FILE'"
log "backup created: $(du -h "$BACKUP_FILE" | cut -f1)"

# 2. Compress
gzip "$BACKUP_FILE"
log "compressed: $(du -h "$COMPRESSED_FILE" | cut -f1)"

# 3. Upload to S3
log "uploading to s3://${BACKUP_S3_BUCKET}/${S3_KEY}"
aws s3 cp "$COMPRESSED_FILE" "s3://${BACKUP_S3_BUCKET}/${S3_KEY}" \
  --endpoint-url "$BACKUP_S3_ENDPOINT" \
  --region "$BACKUP_S3_REGION" \
  --no-progress \
  --quiet
log "upload complete"

# 4. Clean up local temp file
rm -f "$COMPRESSED_FILE"

# 5. Prune old backups (keep RETENTION_DAYS days)
CUTOFF_DATE="$(date -u -d "-${RETENTION_DAYS} days" '+%Y-%m-%d' 2>/dev/null || date -u -v"-${RETENTION_DAYS}d" '+%Y-%m-%d')"
log "pruning backups older than $CUTOFF_DATE (${RETENTION_DAYS} days)"

aws s3api list-objects-v2 \
  --bucket "$BACKUP_S3_BUCKET" \
  --prefix "$PREFIX/" \
  --endpoint-url "$BACKUP_S3_ENDPOINT" \
  --region "$BACKUP_S3_REGION" \
  --query "Contents[?LastModified<='${CUTOFF_DATE}'].Key" \
  --output text 2>/dev/null | tr '\t' '\n' | while read -r key; do
  if [ -n "$key" ] && [ "$key" != "None" ]; then
    log "deleting old backup: $key"
    aws s3 rm "s3://${BACKUP_S3_BUCKET}/${key}" \
      --endpoint-url "$BACKUP_S3_ENDPOINT" \
      --region "$BACKUP_S3_REGION" \
      --quiet
  fi
done

# 6. Verify: list recent backups
BACKUP_COUNT="$(aws s3api list-objects-v2 \
  --bucket "$BACKUP_S3_BUCKET" \
  --prefix "$PREFIX/" \
  --endpoint-url "$BACKUP_S3_ENDPOINT" \
  --region "$BACKUP_S3_REGION" \
  --query "length(Contents)" \
  --output text 2>/dev/null || echo "?")"
log "OK: backup complete. ${BACKUP_COUNT} backups in bucket."
