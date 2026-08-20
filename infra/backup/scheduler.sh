#!/bin/bash
set -eo pipefail

echo "Starting Postgres automated backup scheduler (Daily at 02:00 UTC)..."

while true; do
    echo "Running scheduled database backup at $(date -u +"%Y-%m-%dT%H:%M:%SZ")..."
    /backups/backup.sh || true
    # Sleep 24 hours (86400 seconds)
    sleep 86400
done
