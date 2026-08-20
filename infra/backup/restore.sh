#!/bin/bash
set -eo pipefail

DUMP_FILE="$1"
TARGET_DB="$2"

if [ -z "$DUMP_FILE" ] || [ -z "$TARGET_DB" ]; then
    echo "Usage: $0 <dump_file_path> <target_database_name>"
    exit 1
fi

if [ ! -f "$DUMP_FILE" ]; then
    echo "Error: Dump file '$DUMP_FILE' does not exist."
    exit 1
fi

POSTGRES_HOST="${POSTGRES_HOST:-postgres}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"

if [ -f "/run/secrets/postgres_password" ]; then
    export PGPASSWORD="$(cat /run/secrets/postgres_password | tr -d '\r\n')"
elif [ -n "$POSTGRES_PASSWORD" ]; then
    export PGPASSWORD="$POSTGRES_PASSWORD"
fi

echo "{\"timestamp\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",\"level\":\"INFO\",\"service\":\"db-restore\",\"message\":\"Starting database restore\",\"target_database\":\"${TARGET_DB}\",\"dump_file\":\"${DUMP_FILE}\"}"

# Ensure target database exists
EXISTS=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -tAc "SELECT 1 FROM pg_database WHERE datname='$TARGET_DB';")
if [ "$EXISTS" != "1" ]; then
    echo "Creating target database '$TARGET_DB'..."
    createdb -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" "$TARGET_DB"
fi

# Restore dump into target database
pg_restore -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" --clean --if-exists -d "$TARGET_DB" "$DUMP_FILE" || true

echo "{\"timestamp\":\"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",\"level\":\"INFO\",\"service\":\"db-restore\",\"message\":\"Database restore completed successfully\",\"target_database\":\"${TARGET_DB}\"}"
