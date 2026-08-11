# ADR-0005: Telegram Session Storage via Environment Variable at V1

Status: Accepted  
Date: 2026-08-11

## Context

The Telegram MTProto session (`StringSession`) represents the most sensitive credential in the ingestion listener service. Compromise of the session credential allows full account impersonation and unauthorized API access.

We need a secure, operational, and maintainable method to supply the Telethon session string to the listener service without risking accidental exposure in version control, disk images, or continuous integration logs.

## Decision

At V1 (single-operator, single-environment deployment scale), the Telegram session string is stored and passed strictly via the `TELEGRAM_SESSION_STRING` environment variable:
1. **Local Development**: Injected via a gitignored `.env` file into `docker-compose` or local process environments.
2. **Deployed Environments**: Injected via the hosting platform's encrypted secret configuration mechanisms.
3. **No File Persistence**: The session is never serialized to a `.session` SQLite file inside the repository, committed files, or Docker container images.
4. **Architecture Evolution**: A dedicated centralized secrets manager (e.g., HashiCorp Vault or AWS Secrets Manager) is deferred to V9 per the system roadmap.

## Consequences

- **Security**: Zero risk of committing session binary files or SQLite session databases to Git.
- **Simplicity**: Session rotation is accomplished by updating a single environment variable and restarting the listener container.
- **Testing**: Ingestion logic, fixture validation, and database normalization tests run entirely in CI without needing or loading real session strings.
- **Revisit Trigger**: If the production hosting platform cannot provide encryption-at-rest for environment variable configurations, or when multi-operator credential segregation is required (V9), this decision must be revisited immediately.
