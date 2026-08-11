"""
Configuration management for the Telegram Ingestion Listener.
Loads and validates settings from environment variables.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path


def load_dotenv(filepath: str | Path = ".env") -> None:
    """Simple built-in .env parser to load local environment variables if present."""
    p = Path(filepath)
    if not p.is_file():
        return
    with open(p, encoding="utf-8") as f:
        for raw_line in f:
            line = raw_line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k = k.strip()
            v = v.strip().strip("'\"")
            if k and k not in os.environ:
                os.environ[k] = v


@dataclass(frozen=True)
class Config:
    database_url: str
    telegram_api_id: int
    telegram_api_hash: str
    telegram_session_string: str
    telegram_channels: list[str | int] = field(default_factory=list)
    reconnect_delay_seconds: int = 5
    max_reconnect_retries: int = 10
    log_level: str = "INFO"

    @classmethod
    def from_env(
        cls,
        env: dict[str, str] | None = None,
        allow_empty_session: bool = False,
    ) -> Config:
        """
        Load and validate configuration from the provided environment dictionary or os.environ.
        Raises ValueError if required settings are missing or invalid.
        """
        if env is None:
            load_dotenv()
            environ = os.environ
        else:
            environ = env

        # 1. Database Connection URL
        database_url = environ.get("APP_DATABASE_URL") or environ.get("DATABASE_URL")
        if not database_url:
            user = environ.get("APP_DB_USER") or environ.get("POSTGRES_USER", "postgres")
            password = environ.get("APP_DB_PASSWORD") or environ.get(
                "POSTGRES_PASSWORD", "postgres"
            )
            host = environ.get("POSTGRES_HOST", "localhost")
            port = environ.get("POSTGRES_PORT", "5432")
            dbname = environ.get("POSTGRES_DB", "efi_dev")
            database_url = f"postgres://{user}:{password}@{host}:{port}/{dbname}?sslmode=disable"

        # 2. Telegram API Credentials
        raw_api_id = environ.get("TELEGRAM_API_ID")
        if not raw_api_id or not raw_api_id.strip():
            raise ValueError("Missing required environment variable: TELEGRAM_API_ID")

        try:
            telegram_api_id = int(raw_api_id.strip())
        except ValueError as err:
            raise ValueError(
                f"TELEGRAM_API_ID must be a valid integer, got '{raw_api_id}'"
            ) from err

        telegram_api_hash = environ.get("TELEGRAM_API_HASH", "").strip()
        if not telegram_api_hash:
            raise ValueError("Missing required environment variable: TELEGRAM_API_HASH")

        # 3. Telegram Session String
        telegram_session_string = environ.get("TELEGRAM_SESSION_STRING", "").strip()
        if not telegram_session_string and not allow_empty_session:
            raise ValueError(
                "Missing required environment variable: TELEGRAM_SESSION_STRING (per ADR-0005)"
            )

        # 4. Telegram Channels
        raw_channels = environ.get("TELEGRAM_CHANNELS", "").strip()
        channels: list[str | int] = []
        if raw_channels:
            for item in raw_channels.split(","):
                cleaned = item.strip()
                if not cleaned:
                    continue
                # If numeric channel ID (e.g. -1001234567890 or 1001234567890)
                try:
                    channels.append(int(cleaned))
                except ValueError:
                    # Strip leading @ if present for consistent handle naming
                    if cleaned.startswith("@"):
                        cleaned = cleaned[1:]
                    channels.append(cleaned)

        reconnect_delay = int(environ.get("RECONNECT_DELAY_SECONDS", "5"))
        max_retries = int(environ.get("MAX_RECONNECT_RETRIES", "10"))
        log_level = environ.get("LOG_LEVEL", "INFO").upper()

        return cls(
            database_url=database_url,
            telegram_api_id=telegram_api_id,
            telegram_api_hash=telegram_api_hash,
            telegram_session_string=telegram_session_string,
            telegram_channels=channels,
            reconnect_delay_seconds=reconnect_delay,
            max_reconnect_retries=max_retries,
            log_level=log_level,
        )
