"""
Unit tests for services/listener/backfill.py (V9.2).
Tests bounded message limits, 48-hour cutoff, FloodWaitError resilience,
fresh channel baseline setup, duplicate idempotency, and channel failure isolation.
"""

from datetime import UTC, datetime, timedelta
from typing import Any
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from telethon import errors

from services.listener.backfill import ChannelBackfiller
from services.listener.config import Config


def _create_mock_message(
    msg_id: int,
    text: str = "Test message",
    date: datetime | None = None,
) -> MagicMock:
    msg = MagicMock()
    msg.id = msg_id
    msg.message = text
    msg.text = text
    msg.date = date or datetime.now(UTC)
    msg.entities = None
    msg.media = None
    return msg


@pytest.fixture
def mock_config() -> Config:
    return Config(
        database_url="postgres://user:pass@localhost:5432/db",
        telegram_api_id=123,
        telegram_api_hash="hash",
        telegram_session_string="sess",
        backfill_max_messages=100,
        backfill_max_hours=48,
    )


@pytest.mark.asyncio
async def test_backfill_bounded_message_count(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True
    mock_db.get_channel_last_synced_id.return_value = 50
    mock_db.insert_raw_post.return_value = 1

    mock_client = AsyncMock()

    # Generate 150 messages
    all_msgs = [
        _create_mock_message(
            msg_id=i,
            text=f"Post {i}",
            date=datetime.now(UTC) - timedelta(minutes=i),
        )
        for i in range(150, 0, -1)
    ]

    async def fake_iter_messages(entity: Any, min_id: int = 0, limit: int = 100):
        for msg in all_msgs[:limit]:
            yield msg

    mock_client.iter_messages = fake_iter_messages

    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)
    res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())

    assert res["status"] == "success"
    assert res["fetched"] == 100
    assert res["inserted"] == 100
    assert mock_db.insert_raw_post.call_count == 100


@pytest.mark.asyncio
async def test_backfill_bounded_time_cutoff(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True
    mock_db.get_channel_last_synced_id.return_value = 10
    mock_db.insert_raw_post.return_value = 1

    mock_client = AsyncMock()

    recent_msg = _create_mock_message(
        msg_id=12,
        text="Recent post",
        date=datetime.now(UTC) - timedelta(hours=2),
    )
    old_msg = _create_mock_message(
        msg_id=11,
        text="Ancient post",
        date=datetime.now(UTC) - timedelta(hours=50),
    )

    async def fake_iter_messages(entity: Any, min_id: int = 0, limit: int = 100):
        yield recent_msg
        yield old_msg

    mock_client.iter_messages = fake_iter_messages

    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)
    res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())

    assert res["status"] == "success"
    # Old message was older than 48 hours, so only 1 message is evaluated and inserted
    assert res["fetched"] == 1
    assert res["inserted"] == 1
    assert mock_db.insert_raw_post.call_count == 1
    assert mock_db.insert_raw_post.call_args[1]["telegram_message_id"] == 12


@pytest.mark.asyncio
async def test_backfill_fresh_channel_starts_fresh_without_history_dump(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True
    # Channel has no previous checkpoint
    mock_db.get_channel_last_synced_id.return_value = None

    mock_client = AsyncMock()
    latest_msg = _create_mock_message(msg_id=777, text="Current post")

    async def fake_iter_messages(entity: Any, limit: int = 1):
        yield latest_msg

    mock_client.iter_messages = fake_iter_messages

    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)
    res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())

    assert res["status"] == "fresh_start"
    assert res["fetched"] == 0
    assert res["inserted"] == 0
    # Checkpoint set to current latest message id
    mock_db.update_channel_last_synced_id.assert_called_once_with(1, 777)
    # Zero posts inserted into raw_posts
    assert mock_db.insert_raw_post.call_count == 0


@pytest.mark.asyncio
async def test_backfill_handles_flood_wait_error(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True
    mock_db.get_channel_last_synced_id.return_value = 10
    mock_db.insert_raw_post.return_value = 1

    mock_client = MagicMock()
    msg = _create_mock_message(msg_id=11, text="Post after flood")

    class MockAsyncIterator:
        def __init__(self):
            self.yielded_flood = False
            self.yielded_msg = False

        def __aiter__(self):
            return self

        async def __anext__(self):
            if not self.yielded_flood:
                self.yielded_flood = True
                flood_err = errors.FloodWaitError(request=None)
                flood_err.seconds = 0
                raise flood_err
            if not self.yielded_msg:
                self.yielded_msg = True
                return msg
            raise StopAsyncIteration

    mock_client.iter_messages.return_value = MockAsyncIterator()

    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)

    with patch("asyncio.sleep", new_callable=AsyncMock) as mock_sleep:
        res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())
        mock_sleep.assert_awaited_once_with(0)
        assert res["status"] == "success"
        assert res["fetched"] == 1
        assert res["inserted"] == 1


@pytest.mark.asyncio
async def test_backfill_idempotent_duplicate_handling(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True
    mock_db.get_channel_last_synced_id.return_value = 10
    # First returns ID (inserted), second returns None (duplicate)
    mock_db.insert_raw_post.side_effect = [42, None]

    mock_client = AsyncMock()
    msg1 = _create_mock_message(msg_id=11, text="New post")
    msg2 = _create_mock_message(msg_id=12, text="Duplicate post")

    async def fake_iter_messages(entity: Any, min_id: int = 0, limit: int = 100):
        yield msg2
        yield msg1

    mock_client.iter_messages = fake_iter_messages

    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)
    res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())

    assert res["status"] == "success"
    assert res["fetched"] == 2
    assert res["inserted"] == 1
    assert res["duplicates"] == 1


@pytest.mark.asyncio
async def test_backfill_all_isolates_channel_errors(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = True

    entity1 = MagicMock()
    entity1.id = 101
    entity2 = MagicMock()
    entity2.id = 102

    channel_map = {101: 1, 102: 2}

    mock_client = AsyncMock()
    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)

    # Channel 1 fails with an exception, Channel 2 succeeds
    async def mock_backfill_channel(db_channel_id: int, telegram_entity: Any):
        if db_channel_id == 1:
            raise RuntimeError("Database connection glitch on channel 1")
        return {"status": "success", "fetched": 5, "inserted": 5}

    backfiller.backfill_channel = mock_backfill_channel

    summary = await backfiller.backfill_all(
        channel_map=channel_map,
        resolved_entities=[entity1, entity2],
    )

    assert summary["total_fetched"] == 5
    assert summary["total_inserted"] == 5
    assert len(summary["channels"]) == 2
    assert summary["channels"][0]["result"]["status"] == "failed"
    assert summary["channels"][1]["result"]["status"] == "success"


@pytest.mark.asyncio
async def test_backfill_skips_inactive_channel(mock_config: Config):
    mock_db = MagicMock()
    mock_db.is_channel_active.return_value = False

    mock_client = AsyncMock()
    backfiller = ChannelBackfiller(client=mock_client, db=mock_db, config=mock_config)

    res = await backfiller.backfill_channel(db_channel_id=1, telegram_entity=MagicMock())
    assert res["status"] == "inactive"
    assert res["fetched"] == 0
    assert res["inserted"] == 0
    assert mock_db.get_channel_last_synced_id.call_count == 0
