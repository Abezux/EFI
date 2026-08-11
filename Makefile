.PHONY: up down migrate test test-python lint lint-python format-python validate

# Start local PostgreSQL and listener containers
up:
	docker compose -f infra/docker-compose.yml up -d

# Stop local containers
down:
	docker compose -f infra/docker-compose.yml down

# Apply database migrations
migrate:
	@echo "Applying database migrations..."
	@which psql >/dev/null 2>&1 || (echo "psql not found on PATH. Use docker exec or install postgresql-client." && exit 1)
	PGPASSWORD=$${POSTGRES_PASSWORD:-postgres} psql -h $${POSTGRES_HOST:-localhost} -p $${POSTGRES_PORT:-5432} -U $${POSTGRES_USER:-postgres} -d $${POSTGRES_DB:-efi_dev} -v ON_ERROR_STOP=1 -f db/migrations/0001_init.sql
	@if [ -f db/migrations/0002_seed_channels.sql ]; then \
		PGPASSWORD=$${POSTGRES_PASSWORD:-postgres} psql -h $${POSTGRES_HOST:-localhost} -p $${POSTGRES_PORT:-5432} -U $${POSTGRES_USER:-postgres} -d $${POSTGRES_DB:-efi_dev} -v ON_ERROR_STOP=1 -f db/migrations/0002_seed_channels.sql; \
	fi

# Run Python unit tests
test-python:
	@echo "Running Python listener tests..."
	@if [ -f .venv/bin/pytest ]; then \
		.venv/bin/pytest services/listener/tests/; \
	else \
		pytest services/listener/tests/; \
	fi

# Run Python linter and formatter checks
lint-python:
	@echo "Running Python linter (ruff & black)..."
	@if [ -f .venv/bin/ruff ]; then \
		.venv/bin/ruff check services/listener/ && .venv/bin/black --check services/listener/; \
	else \
		ruff check services/listener/ && black --check services/listener/; \
	fi

# Auto-format Python code
format-python:
	@echo "Formatting Python code..."
	@if [ -f .venv/bin/black ]; then \
		.venv/bin/black services/listener/ && .venv/bin/ruff check --fix services/listener/; \
	else \
		black services/listener/ && ruff check --fix services/listener/; \
	fi

# Run all tests (Go, Python, Fixtures)
test: test-python
	@echo "Running fixture and Go tests..."
	@python3 tests/validate_fixtures.py
	@which go >/dev/null 2>&1 && go test -v ./... || true

# Run all lint checks
lint: lint-python
	@echo "Running Go linter..."
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed locally; will run in CI."

# Run full local validation
validate: lint test
