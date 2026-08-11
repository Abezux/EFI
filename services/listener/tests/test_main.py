"""
Unit tests for services/listener/main.py
"""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from services.listener.config import Config
from services.listener.main import TelegramListener, log_json


def test_log_json_structure(capsys):
    log_json("INFO", "Test message", correlation_id="test-uuid-123", extra={"custom_field": "val"})
    captured = capsys.readouterr()
    line = captured.out.strip()
    data = json.loads(line)

    assert data["level"] == "INFO"
    assert data["service"] == "telegram-listener"
    assert data["message"] == "Test message"
    assert data["correlation_id"] == "test-uuid-123"
    assert data["custom_field"] == "val"
    assert "timestamp" in data


@pytest.mark.asyncio
async def test_handle_new_message_success():
    config = Config(
        database_url="postgres://user:pass@localhost:5432/db",
        telegram_api_id=123,
        telegram_api_hash="hash",
        telegram_session_string="sess",
    )
    mock_db = MagicMock()
    mock_db.insert_raw_post.return_value = 101

    listener = TelegramListener(config, mock_db)
    listener.channel_map = {12345: 1}

    mock_event = MagicMock()
    mock_event.chat_id = 12345
    mock_event.chat.id = 12345
    mock_event.message.id = 999
    mock_event.message.message = "Breaking news"
    mock_event.message.date = None
    mock_event.message.entities = None
    mock_event.message.media = None

    await listener.handle_new_message(mock_event)

    mock_db.insert_raw_post.assert_called_once()
    assert mock_db.insert_raw_post.call_args[1]["channel_id"] == 1
    assert mock_db.insert_raw_post.call_args[1]["telegram_message_id"] == 999


@pytest.mark.asyncio
async def test_unauthorized_session_raises_permission_error():
    config = Config(
        database_url="postgres://user:pass@localhost:5432/db",
        telegram_api_id=123,
        telegram_api_hash="hash",
        telegram_session_string="sess",
    )
    mock_db = MagicMock()
    listener = TelegramListener(config, mock_db)

    mock_client = AsyncMock()
    mock_client.is_user_authorized.return_value = False

    listener.client = mock_client

    with pytest.raises(PermissionError, match="Telegram session unauthorized"):
        await listener.start()
