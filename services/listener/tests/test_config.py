"""
Unit tests for configuration loading and validation in services/listener/config.py
"""

import pytest

from services.listener.config import Config


def test_valid_config_from_env():
    env = {
        "DATABASE_URL": "postgres://efi_app:pass@localhost:5432/efi_dev",
        "TELEGRAM_API_ID": "123456",
        "TELEGRAM_API_HASH": "abcdef1234567890abcdef1234567890",
        "TELEGRAM_SESSION_STRING": "1BVtsOMQBu8x...session_string...",
        "TELEGRAM_CHANNELS": "@ena_news, @fana_broadcast, -1001234567890",
    }
    cfg = Config.from_env(env)
    assert cfg.database_url == "postgres://efi_app:pass@localhost:5432/efi_dev"
    assert cfg.telegram_api_id == 123456
    assert cfg.telegram_api_hash == "abcdef1234567890abcdef1234567890"
    assert cfg.telegram_session_string == "1BVtsOMQBu8x...session_string..."
    assert cfg.telegram_channels == ["ena_news", "fana_broadcast", -1001234567890]
    assert cfg.reconnect_delay_seconds == 5
    assert cfg.max_reconnect_retries == 10
    assert cfg.log_level == "INFO"


def test_missing_api_id_raises_value_error():
    env = {
        "TELEGRAM_API_HASH": "hash123",
        "TELEGRAM_SESSION_STRING": "session123",
    }
    with pytest.raises(ValueError, match="Missing required environment variable: TELEGRAM_API_ID"):
        Config.from_env(env)


def test_invalid_api_id_raises_value_error():
    env = {
        "TELEGRAM_API_ID": "not_an_int",
        "TELEGRAM_API_HASH": "hash123",
        "TELEGRAM_SESSION_STRING": "session123",
    }
    with pytest.raises(ValueError, match="TELEGRAM_API_ID must be a valid integer"):
        Config.from_env(env)


def test_missing_api_hash_raises_value_error():
    env = {
        "TELEGRAM_API_ID": "123456",
        "TELEGRAM_SESSION_STRING": "session123",
    }
    with pytest.raises(
        ValueError, match="Missing required environment variable: TELEGRAM_API_HASH"
    ):
        Config.from_env(env)


def test_missing_session_string_raises_value_error():
    env = {
        "TELEGRAM_API_ID": "123456",
        "TELEGRAM_API_HASH": "hash123",
    }
    with pytest.raises(
        ValueError,
        match="Missing required environment variable: TELEGRAM_SESSION_STRING",
    ):
        Config.from_env(env)


def test_fallback_database_url():
    env = {
        "POSTGRES_USER": "testuser",
        "POSTGRES_PASSWORD": "testpassword",
        "POSTGRES_HOST": "dbhost",
        "POSTGRES_PORT": "5433",
        "POSTGRES_DB": "testdb",
        "TELEGRAM_API_ID": "123456",
        "TELEGRAM_API_HASH": "hash123",
        "TELEGRAM_SESSION_STRING": "session123",
    }
    cfg = Config.from_env(env)
    assert cfg.database_url == "postgres://testuser:testpassword@dbhost:5433/testdb?sslmode=disable"
