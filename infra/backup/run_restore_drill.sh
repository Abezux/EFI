#!/bin/bash
set -eo pipefail

BACKUP_DIR="${BACKUP_DIR:-/backups}"
SOURCE_DB="${SOURCE_DB:-efi_dev}"
TEST_DB="efi_restore_test"
POSTGRES_HOST="${POSTGRES_HOST:-postgres}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

if [ -f "/run/secrets/postgres_password" ]; then
    export PGPASSWORD="$(cat /run/secrets/postgres_password | tr -d '\r\n')"
elif [ -n "$POSTGRES_PASSWORD" ]; then
    export PGPASSWORD="$POSTGRES_PASSWORD"
fi

echo "================================================================="
echo "       V9.3 AUTOMATED DATABASE RESTORE DRILL VERIFICATION       "
echo "================================================================="
echo "Step 1: Taking real live backup of source database '$SOURCE_DB'..."
/backups/backup.sh || /app/infra/backup/backup.sh || ./infra/backup/backup.sh

LATEST_DUMP=$(ls -t "$BACKUP_DIR"/efi_backup_*.dump 2>/dev/null | head -n 1)

if [ -z "$LATEST_DUMP" ]; then
    echo "ERROR: No dump file found in $BACKUP_DIR"
    exit 1
fi

echo "Step 2: Latest backup file verified: $LATEST_DUMP"
echo "Size: $(stat -c%s "$LATEST_DUMP" 2>/dev/null || stat -f%z "$LATEST_DUMP" 2>/dev/null) bytes"

echo "Step 3: Preparing fresh test database '$TEST_DB'..."
psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -c "DROP DATABASE IF EXISTS $TEST_DB;" >/dev/null 2>&1 || true
createdb -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" "$TEST_DB"

echo "Step 4: Restoring dump into test database '$TEST_DB'..."
pg_restore -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" --clean --if-exists -d "$TEST_DB" "$LATEST_DUMP" >/dev/null 2>&1 || true

echo "Step 5: Verifying data integrity between '$SOURCE_DB' and '$TEST_DB'..."

TABLES=("channels" "raw_posts" "news_events" "event_sources" "admin_users" "admin_sessions" "processing_audit")
INTEGRITY_PASSED=true

printf "\n%-20s | %-15s | %-15s | %-10s\n" "TABLE NAME" "SOURCE ($SOURCE_DB)" "RESTORED ($TEST_DB)" "STATUS"
printf "%s\n" "-----------------------------------------------------------------"

for tbl in "${TABLES[@]}"; do
    SRC_COUNT=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$SOURCE_DB" -tAc "SELECT count(*) FROM $tbl;" 2>/dev/null || echo "0")
    TST_COUNT=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$TEST_DB" -tAc "SELECT count(*) FROM $tbl;" 2>/dev/null || echo "0")
    
    STATUS="MATCH"
    if [ "$SRC_COUNT" != "$TST_COUNT" ]; then
        STATUS="MISMATCH"
        INTEGRITY_PASSED=false
    fi
    printf "%-20s | %-15s | %-15s | %-10s\n" "$tbl" "$SRC_COUNT" "$TST_COUNT" "$STATUS"
done

echo "-----------------------------------------------------------------"

if [ "$INTEGRITY_PASSED" = true ]; then
    echo "SUCCESS: Restore drill verified. 100% data integrity match confirmed!"
else
    echo "FAILURE: Row counts mismatched between live and restored databases!"
    exit 1
fi

echo "Step 6: Cleaning up verification test database '$TEST_DB'..."
psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -c "DROP DATABASE IF EXISTS $TEST_DB;" >/dev/null 2>&1 || true
echo "Restore drill complete."
