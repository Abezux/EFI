# Runbook: Telegram Listener Disconnected

## Symptoms
1. **Stale Ingestion**: No new records appear in the `raw_posts` table despite active publishing on monitored Telegram channels.
2. **Log Indicators**:
   - `level`: `ERROR` entries in listener stdout.
   - Message: `Telegram listener disconnected` or `Telegram session unauthorized`.
   - Telethon network errors or connection retry exhaustion.
3. **Container State**: `docker compose ps` shows `listener` container in `restarting` or `exited` status.

---

## Diagnostic Steps

### 1. Check Container Status and Logs
```bash
# Check if the listener process is running
docker compose -f infra/docker-compose.yml ps

# Tail structured JSON logs for the listener
docker compose -f infra/docker-compose.yml logs --tail=100 -f listener
```

### 2. Verify Database Health & Table Counts
Verify that PostgreSQL is healthy and accepting queries from the `efi_app` role:
```bash
PGPASSWORD=efi_app_pass psql -h localhost -p 5432 -U efi_app -d efi_dev -c "
SELECT count(*) AS total_raw_posts, max(posted_at) AS latest_post_time FROM raw_posts;
SELECT id, name, handle, last_seen_at FROM channels ORDER BY last_seen_at DESC NULLS LAST;
"
```

### 3. Identify Common Error Conditions
- **`PermissionError: Telegram session unauthorized`** or **`AuthKeyUnregisteredError`**: The session string has expired, been revoked, or is malformed.
- **`FloodWaitError`**: Telegram API rate limit exceeded. Telethon will log the required wait duration in seconds.
- **`psycopg.OperationalError` / Connection refused**: PostgreSQL container is unreachable or restarting.

---

## Recovery Procedures

### Scenario A: Transient Network Disconnect
Telethon has automatic reconnect with retry backoff enabled. The listener will reconnect automatically once network connectivity is restored. No manual intervention is required unless the process exited after exhausting retries.
```bash
# If container stopped after max retries:
docker compose -f infra/docker-compose.yml restart listener
```

### Scenario B: Telegram Session Revoked or Expired
1. Generate a new valid Telethon session string using your account credentials in a secure offline script.
2. Update the `TELEGRAM_SESSION_STRING` variable in your local `.env` file (or deployment secrets store).
3. Restart the listener container:
```bash
docker compose -f infra/docker-compose.yml restart listener
```
4. Verify logs indicate successful authorization:
```bash
docker compose -f infra/docker-compose.yml logs listener | grep "Telegram client authorized"
```

### Scenario C: FloodWait Rate Limiting
- **DO NOT** restart the listener in a tight loop, as Telegram will increase the cooldown penalty.
- Allow the listener process to sleep for the duration indicated in the `FloodWaitError` log before sending new API requests.

### Scenario D: Database Connectivity Failure
1. Verify PostgreSQL container health:
```bash
docker compose -f infra/docker-compose.yml ps postgres
```
2. If unhealthy, check PostgreSQL logs and restart:
```bash
docker compose -f infra/docker-compose.yml logs postgres
docker compose -f infra/docker-compose.yml restart postgres
docker compose -f infra/docker-compose.yml restart listener
```
