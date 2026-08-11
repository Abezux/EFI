"""
Unit tests for message normalization in services/listener/ingest.py
"""

from datetime import UTC, datetime
from unittest.mock import MagicMock

from services.listener.ingest import (
    extract_entities_from_telethon,
    extract_media_refs_from_telethon,
    normalize_raw_message,
    normalize_telethon_message,
)


def test_normalize_raw_message_success():
    payload = normalize_raw_message(
        channel_id=1,
        message_id=500,
        raw_text="Breaking news from Addis Ababa",
        posted_at="2026-08-11T09:30:00Z",
        raw_entities=[{"type": "bold", "offset": 0, "length": 8}],
        media_refs=[{"type": "photo", "file_id": "photo_123"}],
    )

    assert payload.channel_id == 1
    assert payload.telegram_message_id == 500
    assert payload.raw_text == "Breaking news from Addis Ababa"
    assert payload.processing_status == "ingested"
    assert len(payload.raw_entities) == 1
    assert payload.raw_entities[0]["type"] == "bold"
    assert len(payload.media_refs) == 1
    assert payload.media_refs[0]["file_id"] == "photo_123"
    assert payload.posted_at.year == 2026


def test_normalize_telethon_message():
    mock_entity = MagicMock()
    type(mock_entity).__name__ = "MessageEntityBold"
    mock_entity.offset = 0
    mock_entity.length = 10

    mock_msg = MagicMock()
    mock_msg.id = 777
    mock_msg.message = "Telethon message content"
    mock_msg.date = datetime(2026, 8, 11, 10, 0, 0, tzinfo=UTC)
    mock_msg.entities = [mock_entity]
    mock_msg.media = None

    payload = normalize_telethon_message(
        channel_db_id=5,
        telethon_message=mock_msg,
    )

    assert payload.channel_id == 5
    assert payload.telegram_message_id == 777
    assert payload.raw_text == "Telethon message content"
    assert payload.processing_status == "ingested"
    assert len(payload.raw_entities) == 1
    assert payload.raw_entities[0]["type"] == "MessageEntityBold"
    assert payload.raw_entities[0]["offset"] == 0
    assert payload.raw_entities[0]["length"] == 10
    assert payload.media_refs == []


def test_extract_media_refs_empty_and_dict():
    assert extract_media_refs_from_telethon(None) == []
    assert extract_media_refs_from_telethon([{"type": "photo"}]) == [{"type": "photo"}]


def test_extract_entities_empty():
    assert extract_entities_from_telethon(None) == []
    assert extract_entities_from_telethon([]) == []
