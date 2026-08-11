# Ethiopia News Aggregation Platform

A real-time news aggregation and clustering platform designed to ingest public Telegram channels across Ethiopia, normalize and deduplicate breaking stories, cluster related reports using semantic embeddings (`pgvector`), enrich events with AI summaries and entity metadata, and deliver low-latency updates via Server-Sent Events (SSE).

> **Current Phase: V1 (Telegram Ingestion)**  
> V1 implements the Python/Telethon listener service (`services/listener/`) that ingests messages from configured public Ethiopian Telegram channels into the PostgreSQL `raw_posts` table idempotently, surviving reconnections. No downstream deduplication, embeddings, AI clustering, public APIs, or frontends are active in V1.

---

## Documentation & Architecture

- **System Architecture**: [`docs/architecture/platform-architecture.md`](docs/architecture/platform-architecture.md)
- **V0 Foundation Specification**: [`v0-foundation-spec.md`](v0-foundation-spec.md)
- **V1 Ingestion Specification**: [`v1-ingestion-spec.md`](v1-ingestion-spec.md)
- **Agent Guidelines & Rules**: [`AGENTS.md`](AGENTS.md)
- **Architectural Decision Records (ADRs)**:
  - [ADR-0001: Postgres-Only Queue at V1](docs/adr/0001-postgres-only-queue-at-v1.md)
  - [ADR-0002: Server-Sent Events (SSE) over WebSockets](docs/adr/0002-sse-over-websockets.md)
  - [ADR-0003: pgvector over Dedicated Vector Database](docs/adr/0003-pgvector-over-dedicated-vector-db.md)
  - [ADR-0004: Language Split — Python Listener and Go Processor](docs/adr/0004-language-split-python-listener-go-processor.md)
  - [ADR-0005: Telegram Session Storage via Environment Variable at V1](docs/adr/0005-telegram-session-storage.md)
- **Operational Runbooks**:
  - [Telegram Listener Disconnected](docs/runbooks/telegram-listener-disconnected.md)

---

## Repository Structure

```
/
├── AGENTS.md                          # Mandatory agent and contributor instructions (with V1 addendum)
├── README.md                          # Getting started and repository overview
├── Makefile                           # Local development helper targets
├── pyproject.toml                     # Python test, lint, and formatting configurations
├── go.mod                             # Go module configuration
├── .golangci.yml                      # Go linting ruleset
├── .editorconfig                      # Universal code style configuration
├── .env.example                       # Local environment variable template
├── .gitignore                         # Git exclusion rules
├── .github/
│   └── workflows/
│       └── ci.yml                     # GitHub Actions CI workflow (lint + migrate + test)
├── db/
│   └── migrations/
│       ├── 0001_init.sql              # V1 schema (channels, raw_posts, efi_app role)
│       └── 0002_seed_channels.sql     # Seed public Ethiopian news channels
├── docs/
│   ├── architecture/
│   │   └── platform-architecture.md   # Core system design
│   ├── adr/                           # Architectural Decision Records (0001-0005)
│   └── runbooks/
│       └── telegram-listener-disconnected.md
├── infra/
│   └── docker-compose.yml             # Local PostgreSQL and listener services
├── services/
│   └── listener/                      # Telegram Ingestion Service (Python / Telethon)
│       ├── Dockerfile                 # Container definition (Python 3.11-slim, non-root)
│       ├── requirements.txt           # Pinned dependencies (telethon, psycopg, pytest, ruff, black)
│       ├── config.py                  # Environment variable configuration and validation
│       ├── db.py                      # Parameterized PostgreSQL access layer
│       ├── ingest.py                  # Raw message normalization logic
│       ├── main.py                    # Telethon listener entrypoint & structured JSON logger
│       └── tests/                     # Unit and fixture tests
└── tests/
    ├── fixtures/
    │   └── sample_telegram_posts.json # Sample Telegram posts fixture data
    ├── fixtures_test.go               # Go fixture validation test
    └── validate_fixtures.py           # Python fixture validation script
```

---

## Getting Started

Follow these steps on a clean clone to set up the local development environment and run tests:

### 1. Environment Setup
Copy the environment template to create your local `.env` file:
```bash
cp .env.example .env
```

To run the live listener against Telegram MTProto, populate `TELEGRAM_API_ID`, `TELEGRAM_API_HASH`, and `TELEGRAM_SESSION_STRING` in your `.env` (obtain credentials from [https://my.telegram.org](https://my.telegram.org)).

### 2. Start PostgreSQL (with `pgvector`)
Spin up the local PostgreSQL container using Docker Compose:
```bash
docker compose -f infra/docker-compose.yml up -d postgres
```
*(Alternatively: `make up`)*

### 3. Apply Database Migrations
Apply the schema and channel seed migrations:
```bash
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d efi_dev -v ON_ERROR_STOP=1 -f db/migrations/0001_init.sql
PGPASSWORD=postgres psql -h localhost -p 5432 -U postgres -d efi_dev -v ON_ERROR_STOP=1 -f db/migrations/0002_seed_channels.sql
```
*(Alternatively: `make migrate`)*

This creates:
- `vector` extension
- `channels` table (seeded with initial public channels)
- `raw_posts` table (with `UNIQUE(channel_id, telegram_message_id)` idempotency constraint)
- `efi_app` least-privilege application role with appropriate grants

### 4. Run Automated Tests & Linters
Run the complete automated test suite (no real Telegram credentials required):

**Using Makefile:**
```bash
make validate
```

**Using Python Virtual Environment directly:**
```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r services/listener/requirements.txt
pytest services/listener/tests/
ruff check services/listener/
black --check services/listener/
```

### 5. Running the Listener Service (Live Credentials Required)
Once valid Telegram credentials are configured in `.env`, run the listener:

**Via Docker Compose:**
```bash
docker compose -f infra/docker-compose.yml up --build listener
```

**Via Local Python Process:**
```bash
python -m services.listener.main
```

### 6. Stop Services
When finished, stop all running containers:
```bash
docker compose -f infra/docker-compose.yml down
```
*(Alternatively: `make down`)*
