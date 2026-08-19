"""
Backfill and Gap Detection for the Telegram Ingestion Listener (V9.2).
Fetches missed messages using Telethon's iter_messages upon startup and reconnect.
Reuses existing normalization and idempotent database insertion from V1.
"""

from __future__ import annotations

import asyncio
from datetime import UTC, datetime, timedelta
from typing import Any

from telethon import TelegramClient, errors, utils

from services.listener.config import Config
from services.listener.db import Database
from services.listener.ingest import normalize_telethon_message

SERVICE_NAME = "telegram-listener"


def _log_json(
    level: str,
    message: str,
    extra: dict[str, Any] | None = None,
) -> None:
    """Helper to emit structured JSON logs."""
    from services.listener.main import log_json

    log_json(level=level, message=message, extra=extra)


class ChannelBackfiller:
    def __init__(self, client: TelegramClient, db: Database, config: Config):
        self.client = client
        self.db = db
        self.config = config

    async def backfill_channel(
        self,
        db_channel_id: int,
        telegram_entity: Any,
    ) -> dict[str, Any]:
        """
        Performs bounded backfill for a single channel.
        - Brand-new channels (last_synced_id is None) establish baseline.
        - Existing channels fetch messages newer than last_synced_id up to
          backfill_max_messages or backfill_max_hours.
        - Catches FloodWaitError, waits required duration, and resumes.
        """
        if not self.db.is_channel_active(db_channel_id):
            _log_json(
                "INFO",
                f"Skipping backfill for inactive channel {db_channel_id}",
                extra={"channel_id": db_channel_id},
            )
            return {"status": "inactive", "fetched": 0, "inserted": 0}

        last_synced_id = self.db.get_channel_last_synced_id(db_channel_id)

        # Brand-new channel: do NOT dump full history. Establish baseline from latest message.
        if last_synced_id is None:
            try:
                latest_msg = None
                async for msg in self.client.iter_messages(telegram_entity, limit=1):
                    latest_msg = msg
                    break

                if latest_msg and getattr(latest_msg, "id", None) is not None:
                    self.db.update_channel_last_synced_id(db_channel_id, latest_msg.id)
                    _log_json(
                        "INFO",
                        f"Established baseline checkpoint for fresh channel {db_channel_id} "
                        f"at message {latest_msg.id}",
                        extra={
                            "channel_id": db_channel_id,
                            "checkpoint_message_id": latest_msg.id,
                        },
                    )
            except errors.FloodWaitError as flood_err:
                _log_json(
                    "WARN",
                    f"FloodWaitError during baseline checkpoint for channel {db_channel_id}: "
                    f"waiting {flood_err.seconds}s",
                    extra={"channel_id": db_channel_id, "flood_wait_seconds": flood_err.seconds},
                )
                await asyncio.sleep(flood_err.seconds)
            except Exception as err:
                _log_json(
                    "WARN",
                    f"Could not establish initial checkpoint for channel {db_channel_id}: {err}",
                    extra={"channel_id": db_channel_id, "error": str(err)},
                )

            return {"status": "fresh_start", "fetched": 0, "inserted": 0}

        cutoff_time = datetime.now(UTC) - timedelta(hours=self.config.backfill_max_hours)
        fetched_count = 0
        inserted_count = 0
        duplicate_count = 0

        _log_json(
            "INFO",
            f"Starting bounded backfill for channel {db_channel_id} "
            f"(min_id={last_synced_id}, max_msgs={self.config.backfill_max_messages}, "
            f"cutoff={cutoff_time.isoformat()})",
            extra={
                "channel_id": db_channel_id,
                "min_id": last_synced_id,
                "max_messages": self.config.backfill_max_messages,
                "max_hours": self.config.backfill_max_hours,
            },
        )

        messages_to_process: list[Any] = []
        try:
            # We iterate messages with retry for flood waits
            iterator = self.client.iter_messages(
                telegram_entity,
                min_id=last_synced_id,
                limit=self.config.backfill_max_messages,
            )

            while True:
                try:
                    msg = await iterator.__anext__()
                    messages_to_process.append(msg)
                except StopAsyncIteration:
                    break
                except errors.FloodWaitError as flood_err:
                    _log_json(
                        "WARN",
                        f"FloodWaitError during iter_messages for channel {db_channel_id}: "
                        f"waiting {flood_err.seconds}s",
                        extra={
                            "channel_id": db_channel_id,
                            "flood_wait_seconds": flood_err.seconds,
                        },
                    )
                    await asyncio.sleep(flood_err.seconds)
                    continue

        except Exception as iter_err:
            _log_json(
                "ERROR",
                f"Error retrieving backfill messages for channel {db_channel_id}: {iter_err}",
                extra={"channel_id": db_channel_id, "error": str(iter_err)},
            )
            return {
                "status": "partial_error",
                "fetched": fetched_count,
                "inserted": inserted_count,
                "error": str(iter_err),
            }

        # Process retrieved messages (Telethon returns newest to oldest)
        # Reverse to process oldest to newest so checkpoints advance naturally
        messages_to_process.reverse()

        for msg in messages_to_process:
            msg_id = getattr(msg, "id", None)
            if msg_id is None:
                continue

            msg_date = getattr(msg, "date", None)
            if msg_date is not None:
                if msg_date.tzinfo is None:
                    msg_date = msg_date.replace(tzinfo=UTC)
                if msg_date < cutoff_time:
                    # Message is older than the 48-hour recency window
                    continue

            # Skip messages with no text and no media
            if not getattr(msg, "text", None) and not getattr(msg, "media", None):
                continue

            fetched_count += 1
            try:
                payload = normalize_telethon_message(
                    channel_db_id=db_channel_id,
                    telethon_message=msg,
                )

                raw_post_id = self.db.insert_raw_post(
                    channel_id=payload.channel_id,
                    telegram_message_id=payload.telegram_message_id,
                    raw_text=payload.raw_text,
                    raw_entities=payload.raw_entities,
                    media_refs=payload.media_refs,
                    posted_at=payload.posted_at,
                )

                if raw_post_id is not None:
                    inserted_count += 1
                    _log_json(
                        "INFO",
                        f"Ingested backfilled message {payload.telegram_message_id} "
                        f"from channel {db_channel_id}",
                        extra={
                            "raw_post_id": raw_post_id,
                            "channel_id": db_channel_id,
                            "telegram_message_id": payload.telegram_message_id,
                            "status": "backfilled",
                        },
                    )
                else:
                    duplicate_count += 1
            except Exception as err:
                _log_json(
                    "ERROR",
                    f"Failed to ingest backfilled message {msg_id} "
                    f"from channel {db_channel_id}: {err}",
                    extra={
                        "channel_id": db_channel_id,
                        "telegram_message_id": msg_id,
                        "error": str(err),
                    },
                )

        _log_json(
            "INFO",
            f"Backfill finished for channel {db_channel_id}: {fetched_count} evaluated, "
            f"{inserted_count} new inserted, {duplicate_count} duplicate(s)",
            extra={
                "channel_id": db_channel_id,
                "fetched": fetched_count,
                "inserted": inserted_count,
                "duplicates": duplicate_count,
            },
        )

        return {
            "status": "success",
            "fetched": fetched_count,
            "inserted": inserted_count,
            "duplicates": duplicate_count,
        }

    async def backfill_all(
        self,
        channel_map: dict[int, int],
        resolved_entities: list[Any],
    ) -> dict[str, Any]:
        """
        Runs backfill across all configured and active channels.
        Isolates per-channel failures so one channel error does not abort the entire run.
        """
        _log_json("INFO", f"Initiating backfill across {len(resolved_entities)} channel(s)...")

        total_fetched = 0
        total_inserted = 0
        channel_summaries: list[dict[str, Any]] = []

        for entity in resolved_entities:
            telegram_channel_id = getattr(entity, "id", None)
            peer_id = None
            try:
                if entity:
                    peer_id = utils.get_peer_id(entity)
            except Exception:
                peer_id = None

            db_channel_id = (
                channel_map.get(telegram_channel_id)
                if telegram_channel_id is not None
                else None
            ) or (
                channel_map.get(peer_id)
                if peer_id is not None
                else None
            )

            if not db_channel_id:
                continue

            try:
                res = await self.backfill_channel(
                    db_channel_id=db_channel_id,
                    telegram_entity=entity,
                )
                total_fetched += res.get("fetched", 0)
                total_inserted += res.get("inserted", 0)
                channel_summaries.append({"channel_id": db_channel_id, "result": res})
            except Exception as err:
                _log_json(
                    "ERROR",
                    f"Unhandled error during backfill on channel {db_channel_id}: {err}",
                    extra={"channel_id": db_channel_id, "error": str(err)},
                )
                channel_summaries.append({
                    "channel_id": db_channel_id,
                    "result": {"status": "failed", "error": str(err)},
                })

        _log_json(
            "INFO",
            f"Completed global backfill: {total_fetched} messages evaluated, "
            f"{total_inserted} inserted",
            extra={"total_fetched": total_fetched, "total_inserted": total_inserted},
        )

        return {
            "total_fetched": total_fetched,
            "total_inserted": total_inserted,
            "channels": channel_summaries,
        }
