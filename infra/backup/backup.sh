#!/bin/bash
set -eo pipefail

BACKUP_DIR="${BACKUP_DIR:-/backups}"
POSTGRES_HOST="${POSTGRES_HOST:-postgres}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-efi_dev}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

# Resolve password from secret file if present
if [ -f "/run/secrets/postgres_password" ]; then
    export PGPASSWORD="$(cat /run/secrets/postgres_password | tr -d '\r\n')"
elif [ -n "$POSTGRES_PASSWORD" ]; then
    export PGPASSWORD="$POSTGRES_PASSWORD"
fi

mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date -u +"%Y%m%d_%H%M%S")
FILENAME="efi_backup_${TIMESTAMP}.dump"
FILEPATH="${BACKUP_DIR}/${FILENAME}"

echo "{\"timestamp\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",\"level\":\"INFO\",\"service\":\"db-backup\",\"message\":\"Starting database backup\",\"database\":\"${POSTGRES_DB}\",\"target_file\":\"${FILEPATH}\"}"

pg_dump -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -Fc -f "$FILEPATH" "$POSTGRES_DB"

FILESIZE=$(stat -c%s "$FILEPATH" 2>/dev/null || stat -f%z "$FILEPATH" 2>/dev/null || echo "unknown")

echo "{\"timestamp\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",\"level\":\"INFO\",\"service\":\"db-backup\",\"message\":\"Database backup completed successfully\",\"database\":\"${POSTGRES_DB}\",\"backup_file\":\"${FILEPATH}\",\"size_bytes\":${FILESIZE}}"

# Apply retention policy: delete dumps older than RETENTION_DAYS
echo "{\"timestamp\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",\"level\":\"INFO\",\"service\":\"db-backup\",\"message\":\"Applying backup retention policy (${RETENTION_DAYS} days)\"}"
find "$BACKUP_DIR" -name "efi_backup_*.dump" -mtime +"$RETENTION_DAYS" -exec rm -f {} \;
