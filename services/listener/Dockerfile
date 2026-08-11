# Multi-stage or slim secure Python container for Telegram Ingestion Listener
FROM python:3.11-slim AS runner

# Set environment variables
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app

WORKDIR /app

# Install system dependencies if required for psycopg / networking
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

# Install pinned Python dependencies
COPY services/listener/requirements.txt /app/services/listener/requirements.txt
RUN pip install --no-cache-dir -r /app/services/listener/requirements.txt

# Copy service code
COPY pyproject.toml /app/pyproject.toml
COPY services /app/services

# Create non-root system user for least privilege execution
RUN groupadd -r appuser && useradd -r -g appuser -u 1000 appuser && \
    chown -R appuser:appuser /app

USER appuser

# Run listener service
CMD ["python", "-m", "services.listener.main"]
