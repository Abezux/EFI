# ADR-0016: Secrets Management via Docker Secrets and Automated PostgreSQL Backup & Restore Strategy

## Status
Accepted
Date: 2026-08-20

## Context
In V9.3 (Secrets Hardening & Backups), the Ethiopia News Aggregation Platform requires:
1. Hardening secret credentials (`TELEGRAM_API_HASH`, `TELEGRAM_SESSION_STRING`, `GEMINI_API_KEY`, and database user passwords for `efi_app`, `efi_api`, and `efi_admin`) by migrating them out of plain text `.env` files into a dedicated secrets management solution.
2. An automated, durable database backup and restore strategy to ensure data resilience against hardware failure, corruption, or catastrophic host loss.

Prior to V9.3:
- ADR-0005 governed storing `TELEGRAM_SESSION_STRING` in environment variables loaded from `.env`.
- Secrets for dev/prod were stored in `.env` files on disk, risking accidental commit or exposure via environment inspection (`docker inspect`, `/proc` access).
- No automated backup or verified restore procedure existed for PostgreSQL.

---

## Decision Proposal

### 1. Secrets Management: Docker Secrets (`/run/secrets/`) + Container Entrypoint Injection
- **Selected Secrets Manager**: Docker Secrets native volume mounts (`/run/secrets/<secret_name>`) managed via Docker Compose (`secrets:` definition block).
- **Runtime Injection Mechanism**: Service entrypoint scripts (`infra/entrypoints/entrypoint.sh`) check for files in `/run/secrets/`. If present, secrets are exported into environment variables (`export SECRET_NAME=$(cat /run/secrets/secret_name)`) before launching service binaries or scripts.
- **Code Decoupling**: Application code in Go, Python, and Next.js remains completely unchanged—services continue to consume standard environment variables (`os.Getenv`, `os.environ.get`, `process.env`).
- **Dev/Production Isolation**:
  - Pure local dev uses `.env` (gitignored).
  - Staging/Production deployments read secrets strictly from restricted files (`infra/secrets/`, permissions `0600`, gitignored) mounted into container memory (`tmpfs` at `/run/secrets/`).
- **Superseding ADR-0005**: ADR-0005's decision to rely on unencrypted `.env` environment files is formally **superseded**. `TELEGRAM_SESSION_STRING` will now be managed via Docker Secrets and injected into memory at container start.

#### Reasoning for Secrets Manager Choice:
1. **Zero Runtime Overhead & No External Vendor Lock-in**: Works out of the box with Docker Engine / Docker Compose on any Linux VM without needing paid external cloud services or running complex third-party vault servers (e.g., HashiCorp Vault) for a lightweight self-hosted deployment.
2. **Process & Inspection Isolation**: Secrets mounted at `/run/secrets/` exist on `tmpfs` in container memory and are hidden from `docker inspect` and host process listings (`ps aux`).
3. **Seamless Seam**: Decouples secret storage from application runtime without modifying service code.

---

### 2. Database Backup & Restore Strategy
- **Backup Utility & Format**: `pg_dump -Fc` (PostgreSQL Custom Dump format) with gzip compression.
- **Schedule**: Daily automated backups at 02:00 UTC via a lightweight dedicated backup sidecar container (`infra/backup/backup.sh`).
- **Retention Policy**:
  - Simple 7-day rolling window (`find /backups -name "*.dump" -mtime +7 -delete`).
- **Off-Host Storage Isolation**:
  - Backup files are written to a dedicated backup directory mounted outside the primary DB volume (`infra/backups/storage/` or S3/remote storage target).
- **Restore Strategy & Automated Verification Drill**:
  - Documented restore script (`infra/backup/restore.sh`) utilizing `pg_restore --clean --if-exists`.
  - The restore drill automatically restores a backup into an isolated test database (`efi_restore_test`), performs validation checks (row counts across `channels`, `raw_posts`, `news_events`, `event_sources`, `admin_users`), and verifies data integrity before tearing down the test database.

#### Reasoning for Backup Strategy Choice:
1. `pg_dump -Fc` produces a compact, compressed binary dump supported natively by PostgreSQL.
2. Custom format enables parallel restore (`pg_restore -j`) and table-level inspection.
3. 7-day retention provides sufficient rollback window for small-scale deployments without storage bloat.

---

## Consequences

### Positive
- **Complete Secret Isolation**: No plaintext secrets in `.env` files in staging/prod environments or host process lists.
- **Zero Application Code Changes**: Microservices retain standard environment variable lookup seams.
- **Data Safety Guarantee**: Daily automated backups and a validated, executable restore script guarantee zero-data-loss recovery capability.

### Trade-offs & Limitations
- Rotating a secret requires updating the secret file in `infra/secrets/` and restarting the relevant service container (`docker compose restart <service>`).
