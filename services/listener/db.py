"""
Minimal database access layer for the Telegram Ingestion Listener.
Uses psycopg 3 with parameterized queries and no ORM.
Enforces least-privilege access and atomic idempotency via PostgreSQL constraints.
"""

from __future__ import annotations

import json
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import datetime
from typing import Any

import psycopg
from psycopg.rows import dict_row


class Database:
    def __init__(self, connection_url: str):
        self.connection_url = connection_url

    @contextmanager
    def get_connection(self) -> Iterator[psycopg.Connection[Any]]:
        """Context manager for acquiring and safely closing a database connection."""
        conn = psycopg.connect(self.connection_url, row_factory=dict_row)
        try:
            yield conn
            conn.commit()
        except Exception:
            conn.rollback()
            raise
        finally:
            conn.close()

    def get_or_create_channel(
        self,
        telegram_channel_id: int,
        name: str,
        handle: str | None = None,
    ) -> int:
        """
        Retrieves an existing channel by telegram_channel_id or inserts a new one.
        Updates last_seen_at timestamp.
        """
        query = """
        INSERT INTO channels (telegram_channel_id, name, handle, is_active, added_at, last_seen_at)
        VALUES (%(telegram_channel_id)s, %(name)s, %(handle)s, true, NOW(), NOW())
        ON CONFLICT (telegram_channel_id) DO UPDATE
        SET last_seen_at = NOW(),
            name = CASE WHEN EXCLUDED.name != '' THEN EXCLUDED.name ELSE channels.name END,
            handle = COALESCE(EXCLUDED.handle, channels.handle)
        RETURNING id;
        """
        with self.get_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    query,
                    {
                        "telegram_channel_id": telegram_channel_id,
                        "name": name,
                        "handle": handle,
                    },
                )
                row = cur.fetchone()
                if row is None:
                    raise RuntimeError(f"Failed to upsert channel {telegram_channel_id}")
                return int(row["id"])

    def insert_raw_post(
        self,
        channel_id: int,
        telegram_message_id: int,
        raw_text: str,
        raw_entities: list[dict[str, Any]] | None = None,
        media_refs: list[dict[str, Any]] | None = None,
        posted_at: datetime | None = None,
    ) -> int | None:
        """
        Inserts an immutable raw post record into raw_posts.
        Relies on UNIQUE(channel_id, telegram_message_id) constraint for idempotency.
        Returns the inserted row ID, or None if the message was already ingested (conflict no-op).
        """
        if posted_at is None:
            posted_at = datetime.now()

        entities_json = json.dumps(raw_entities if raw_entities is not None else [])
        media_json = json.dumps(media_refs if media_refs is not None else [])

        query = """
        INSERT INTO raw_posts (
            channel_id,
            telegram_message_id,
            raw_text,
            raw_entities,
            media_refs,
            posted_at,
            ingested_at,
            processing_status
        ) VALUES (
            %(channel_id)s,
            %(telegram_message_id)s,
            %(raw_text)s,
            %(raw_entities)s::jsonb,
            %(media_refs)s::jsonb,
            %(posted_at)s,
            NOW(),
            'ingested'
        )
        ON CONFLICT (channel_id, telegram_message_id) DO NOTHING
        RETURNING id;
        """
        with self.get_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    query,
                    {
                        "channel_id": channel_id,
                        "telegram_message_id": telegram_message_id,
                        "raw_text": raw_text,
                        "raw_entities": entities_json,
                        "media_refs": media_json,
                        "posted_at": posted_at,
                    },
                )
                row = cur.fetchone()
                if row is None:
                    # Message was already ingested previously (idempotent no-op)
                    return None
                return int(row["id"])

    def is_channel_active(self, channel_id: int) -> bool:
        """Checks if a channel is currently active for message ingestion."""
        query = "SELECT is_active FROM channels WHERE id = %(channel_id)s;"
        with self.get_connection() as conn:
            with conn.cursor() as cur:
                cur.execute(query, {"channel_id": channel_id})
                row = cur.fetchone()
                if row is None:
                    return False
                return bool(row["is_active"])
