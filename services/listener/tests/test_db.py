"""
Unit tests for database interactions in services/listener/db.py
"""

from datetime import UTC, datetime
from unittest.mock import MagicMock, patch

from services.listener.db import Database


def test_insert_raw_post_success():
    mock_cursor = MagicMock()
    mock_cursor.fetchone.return_value = {"id": 42}

    mock_conn = MagicMock()
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    db = Database("postgres://user:pass@localhost:5432/db")

    with patch("psycopg.connect", return_value=mock_conn):
        post_id = db.insert_raw_post(
            channel_id=1,
            telegram_message_id=1001,
            raw_text="Test news post text",
            raw_entities=[{"type": "bold", "offset": 0, "length": 4}],
            media_refs=[{"type": "photo", "file_id": "123.jpg", "caption": "cap"}],
            posted_at=datetime(2026, 8, 11, 12, 0, 0, tzinfo=UTC),
        )

        assert post_id == 42
        assert mock_cursor.execute.call_count == 2
        query, params = mock_cursor.execute.call_args_list[0][0]
        assert "ON CONFLICT (channel_id, telegram_message_id) DO NOTHING" in query
        assert params["channel_id"] == 1
        assert params["telegram_message_id"] == 1001
        assert params["raw_text"] == "Test news post text"
        assert '{"type": "bold"' in params["raw_entities"]
        assert '{"type": "photo"' in params["media_refs"]

        # Second execute updates last_synced_message_id
        update_q, update_p = mock_cursor.execute.call_args_list[1][0]
        assert "UPDATE channels" in update_q
        assert "last_synced_message_id" in update_q
        assert update_p["channel_id"] == 1
        assert update_p["telegram_message_id"] == 1001
        assert mock_conn.commit.called


def test_insert_raw_post_idempotent_duplicate_noop():
    mock_cursor = MagicMock()
    # fetchone returns None when ON CONFLICT DO NOTHING does not insert a row
    mock_cursor.fetchone.return_value = None

    mock_conn = MagicMock()
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    db = Database("postgres://user:pass@localhost:5432/db")

    with patch("psycopg.connect", return_value=mock_conn):
        post_id = db.insert_raw_post(
            channel_id=1,
            telegram_message_id=1001,
            raw_text="Duplicate test post",
        )

        assert post_id is None
        assert mock_cursor.execute.call_count == 2
        assert mock_conn.commit.called


def test_get_or_create_channel():
    mock_cursor = MagicMock()
    mock_cursor.fetchone.return_value = {"id": 7}

    mock_conn = MagicMock()
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    db = Database("postgres://user:pass@localhost:5432/db")

    with patch("psycopg.connect", return_value=mock_conn):
        channel_id = db.get_or_create_channel(
            telegram_channel_id=1001234567890,
            name="ENA News",
            handle="ena_news",
        )

        assert channel_id == 7
        mock_cursor.execute.assert_called_once()
        query, params = mock_cursor.execute.call_args[0]
        assert "INSERT INTO channels" in query
        assert params["telegram_channel_id"] == 1001234567890
        assert params["name"] == "ENA News"
        assert params["handle"] == "ena_news"


def test_get_channel_last_synced_id():
    mock_cursor = MagicMock()
    mock_cursor.fetchone.return_value = {"last_synced_message_id": 555}

    mock_conn = MagicMock()
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    db = Database("postgres://user:pass@localhost:5432/db")

    with patch("psycopg.connect", return_value=mock_conn):
        synced_id = db.get_channel_last_synced_id(1)
        assert synced_id == 555
        mock_cursor.execute.assert_called_once()
        query, params = mock_cursor.execute.call_args[0]
        assert "SELECT last_synced_message_id FROM channels" in query
        assert params["channel_id"] == 1


def test_update_channel_last_synced_id():
    mock_cursor = MagicMock()

    mock_conn = MagicMock()
    mock_conn.cursor.return_value.__enter__.return_value = mock_cursor

    db = Database("postgres://user:pass@localhost:5432/db")

    with patch("psycopg.connect", return_value=mock_conn):
        db.update_channel_last_synced_id(1, 999)
        mock_cursor.execute.assert_called_once()
        query, params = mock_cursor.execute.call_args[0]
        assert "UPDATE channels" in query
        assert "last_synced_message_id" in query
        assert params["channel_id"] == 1
        assert params["message_id"] == 999

