"""
Message normalization logic for Telegram messages.
Transforms raw Telethon messages or fixture dictionaries into structured RawPostPayload records.
No deduplication beyond DB-level idempotency and no AI processing in V1.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any


@dataclass
class RawPostPayload:
    channel_id: int
    telegram_message_id: int
    raw_text: str
    raw_entities: list[dict[str, Any]] = field(default_factory=list)
    media_refs: list[dict[str, Any]] = field(default_factory=list)
    posted_at: datetime = field(default_factory=lambda: datetime.now(UTC))
    processing_status: str = "ingested"


def extract_entities_from_telethon(entities: list[Any] | None) -> list[dict[str, Any]]:
    """Serializes Telethon MessageEntity objects into JSON-compatible dictionaries."""
    if not entities:
        return []

    result: list[dict[str, Any]] = []
    for entity in entities:
        if isinstance(entity, dict):
            result.append(entity)
            continue

        entity_type = type(entity).__name__
        offset = getattr(entity, "offset", 0)
        length = getattr(entity, "length", 0)
        item: dict[str, Any] = {
            "type": entity_type,
            "offset": offset,
            "length": length,
        }

        # Optional url for MessageEntityTextUrl
        if hasattr(entity, "url"):
            item["url"] = entity.url

        result.append(item)
    return result


def extract_media_refs_from_telethon(media: Any) -> list[dict[str, Any]]:
    """Extracts media metadata from a Telethon media object."""
    if not media:
        return []

    if isinstance(media, list):
        return media

    if isinstance(media, dict):
        return [media]

    media_type = type(media).__name__
    item: dict[str, Any] = {"type": media_type}

    # Extract file identifiers or document/photo attributes safely if present
    if hasattr(media, "photo"):
        photo = media.photo
        if photo and hasattr(photo, "id"):
            item["file_id"] = str(photo.id)
    elif hasattr(media, "document"):
        doc = media.document
        if doc:
            if hasattr(doc, "id"):
                item["file_id"] = str(doc.id)
            if hasattr(doc, "mime_type"):
                item["mime_type"] = doc.mime_type
    elif hasattr(media, "webpage"):
        webpage = media.webpage
        if webpage:
            if hasattr(webpage, "url"):
                item["url"] = webpage.url
            if hasattr(webpage, "title"):
                item["title"] = webpage.title

    return [item]


def normalize_raw_message(
    channel_id: int,
    message_id: int,
    raw_text: str,
    posted_at: datetime | str,
    raw_entities: list[Any] | None = None,
    media_refs: list[Any] | None = None,
) -> RawPostPayload:
    """Normalizes raw input parameters into a structured RawPostPayload."""
    if isinstance(posted_at, str):
        parsed_posted_at = datetime.fromisoformat(posted_at.replace("Z", "+00:00"))
    else:
        parsed_posted_at = posted_at

    return RawPostPayload(
        channel_id=channel_id,
        telegram_message_id=message_id,
        raw_text=raw_text,
        raw_entities=extract_entities_from_telethon(raw_entities),
        media_refs=extract_media_refs_from_telethon(media_refs),
        posted_at=parsed_posted_at,
        processing_status="ingested",
    )


def normalize_telethon_message(
    channel_db_id: int,
    telethon_message: Any,
) -> RawPostPayload:
    """Normalizes a live Telethon Message object into a RawPostPayload."""
    raw_text = getattr(telethon_message, "message", "") or getattr(telethon_message, "raw_text", "")
    message_id = int(getattr(telethon_message, "id", 0))
    posted_at = getattr(telethon_message, "date", datetime.now(UTC))

    entities = getattr(telethon_message, "entities", None)
    media = getattr(telethon_message, "media", None)

    return RawPostPayload(
        channel_id=channel_db_id,
        telegram_message_id=message_id,
        raw_text=raw_text,
        raw_entities=extract_entities_from_telethon(entities),
        media_refs=extract_media_refs_from_telethon(media),
        posted_at=posted_at,
        processing_status="ingested",
    )
