# Ethiopia News Aggregator (EFI)

Real-time, multi-source Ethiopian news aggregation, semantic clustering, AI summarization, and live delivery platform.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Python](https://img.shields.io/badge/Python-3.11+-3776AB?style=flat&logo=python)](https://www.python.org)
[![Next.js](https://img.shields.io/badge/Next.js-14-black?style=flat&logo=next.js)](https://nextjs.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16%20%2B%20pgvector-336791?style=flat&logo=postgresql)](https://github.com/pgvector/pgvector)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## About

Ethiopia News Aggregator (EFI) aggregates breaking news reports from public Telegram channels across Ethiopia into unified, cohesive stories. It ingests raw posts in real time, normalizes and deduplicates text using 64-bit Simhash, clusters related multilingual coverage via Google Gemini vector embeddings (`pgvector`), synthesizes neutral multi-source summaries with verbatim quotes using Gemini LLM, and streams live updates to a Next.js web application over Server-Sent Events (SSE).

---

## Project Structure

```
.
├── db/                         # PostgreSQL migration scripts (0001–0010) & migration runner
├── docs/                       # Architectural Decision Records (ADRs) & system documentation
├── infra/                      # Docker Compose multi-service definitions
├── services/
│   ├── listener/               # Telegram MTProto ingestion service (Python / Telethon)
│   ├── processor/              # Simhash deduplication, embedding clustering & LLM enrichment (Go)
│   └── api/                    # REST API, SSE streaming hub & admin moderation service (Go)
├── tests/                      # Shared test fixtures and validation scripts
└── web/                        # Public & admin web application (Next.js App Router / TypeScript)
```

---

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & Docker Compose
- [Go](https://go.dev/) (1.22+) *(optional for local development)*
- [Python](https://www.python.org/) (3.11+) *(optional for local development)*
- [Node.js](https://nodejs.org/) (20+) *(optional for local development)*

### 1. Configure Environment Variables

Copy the example configuration file and fill in required values:

```bash
cp .env.example .env
```

Key environment variables:

| Variable | Description |
| --- | --- |
| `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` | PostgreSQL connection settings |
| `APP_DB_USER`, `APP_DB_PASSWORD` | Database credentials for application services |
| `API_DB_USER`, `API_DB_PASSWORD` | Read-only database credentials for public API |
| `ADMIN_DB_USER`, `ADMIN_DB_PASSWORD` | Administrative database credentials |
| `TELEGRAM_API_ID`, `TELEGRAM_API_HASH`, `TELEGRAM_SESSION_STRING` | Telegram MTProto API client credentials |
| `GEMINI_API_KEY` | Google Gemini API key for embeddings and LLM summaries |
| `ADMIN_INITIAL_USERNAME`, `ADMIN_INITIAL_PASSWORD` | Initial admin account seed credentials |
| `SESSION_SECRET` | Secret key used for signing administrative session cookies |
| `INTERNAL_API_URL`, `NEXT_PUBLIC_API_URL` | API backend endpoints for SSR and client polling |

### 2. Run with Docker Compose

Start the full stack (database, listener, processor, API, web frontend):

```bash
docker compose -f infra/docker-compose.yml up --build -d
```

Services will be accessible at:
- **Web Frontend & Admin**: `http://localhost:3000`
- **REST & SSE API**: `http://localhost:8080`
- **PostgreSQL**: `localhost:5432`

### 3. Run Tests Locally

- **Python Listener**: `pytest services/listener/`
- **Go Processor & API**: `go test ./...` (inside `services/processor` and `services/api`)
- **Web Frontend**: `npm --prefix web test`

---

## License

This project is licensed under the [MIT License](LICENSE).
