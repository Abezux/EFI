"""
Entrypoint for the Telegram Ingestion Listener Service (V1).
Connects to Telegram via Telethon, subscribes to configured channels,
normalizes messages, and stores them in raw_posts idempotently.
Emits structured JSON logs per repository conventions.
"""

from __future__ import annotations

import asyncio
import json
import signal
import sys
import uuid
from datetime import UTC, datetime
from typing import Any, cast

from telethon import TelegramClient, events, utils
from telethon.sessions import StringSession

from services.listener.config import Config
from services.listener.db import Database
from services.listener.ingest import normalize_telethon_message

SERVICE_NAME = "telegram-listener"


def log_json(
    level: str,
    message: str,
    correlation_id: str | None = None,
    extra: dict[str, Any] | None = None,
) -> None:
    """Emits a single-line structured JSON log per AGENTS.md Section 6."""
    entry: dict[str, Any] = {
        "timestamp": datetime.now(UTC).isoformat(),
        "level": level.upper(),
        "service": SERVICE_NAME,
        "message": message,
        "correlation_id": correlation_id or str(uuid.uuid4()),
    }
    if extra:
        entry.update(extra)
    sys.stdout.write(json.dumps(entry, ensure_ascii=False) + "\n")
    sys.stdout.flush()


class TelegramListener:
    def __init__(self, config: Config, db: Database):
        self.config = config
        self.db = db
        self.client: TelegramClient | None = None
        self.running = False
        # Map: Telegram peer/channel ID -> Postgres channels.id
        self.channel_map: dict[int, int] = {}

    async def initialize_client(self) -> None:
        """Initializes the Telethon client with the session string from environment."""
        session = StringSession(self.config.telegram_session_string or "")
        self.client = TelegramClient(
            session,
            self.config.telegram_api_id,
            self.config.telegram_api_hash,
            auto_reconnect=True,
            retry_delay=self.config.reconnect_delay_seconds,
            connection_retries=self.config.max_reconnect_retries,
        )

    async def resolve_channels(self) -> list[Any]:
        """Resolves configured channel identifiers and syncs them with the channels table."""
        if not self.client:
            raise RuntimeError("Telegram client not initialized")

        resolved_entities: list[Any] = []
        for ch in self.config.telegram_channels:
            try:
                entity = await self.client.get_entity(ch)
                resolved_entities.append(entity)

                telegram_channel_id = getattr(entity, "id", None)
                if telegram_channel_id is not None:
                    name = getattr(entity, "title", str(ch))
                    handle = getattr(entity, "username", None)

                    # Upsert channel row into DB
                    db_channel_id = self.db.get_or_create_channel(
                        telegram_channel_id=telegram_channel_id,
                        name=name,
                        handle=handle,
                    )
                    # Store both bare channel ID and full peer ID
                    self.channel_map[telegram_channel_id] = db_channel_id
                    peer_id = utils.get_peer_id(entity)
                    self.channel_map[peer_id] = db_channel_id

                    log_json(
                        "INFO",
                        f"Mapped channel '{name}' -> DB id {db_channel_id}",
                        extra={
                            "telegram_channel_id": telegram_channel_id,
                            "peer_id": peer_id,
                            "db_channel_id": db_channel_id,
                            "handle": handle,
                        },
                    )
            except Exception as err:
                log_json(
                    "ERROR",
                    f"Failed to resolve channel identifier '{ch}': {err}",
                    extra={"channel_identifier": str(ch), "error": str(err)},
                )

        return resolved_entities

    async def handle_new_message(self, event: events.NewMessage.Event) -> None:
        """Handles an incoming message event, normalizes it, and inserts it into raw_posts."""
        correlation_id = str(uuid.uuid4())
        msg = event.message
        chat_id = event.chat_id

        # Resolve peer channel ID
        resolved_id, _ = utils.resolve_id(chat_id)
        db_channel_id = self.channel_map.get(chat_id) or self.channel_map.get(resolved_id)

        chat = event.chat
        if not chat and hasattr(event, "get_chat"):
            try:
                chat = await event.get_chat()
            except Exception:
                chat = None

        if not db_channel_id and chat:
            db_channel_id = self.channel_map.get(chat.id)

        if not db_channel_id:
            # Check if this chat is one of our monitored channels
            chat_name = getattr(chat, "title", f"Channel {resolved_id}")
            chat_handle = getattr(chat, "username", None)
            try:
                db_channel_id = self.db.get_or_create_channel(
                    telegram_channel_id=resolved_id,
                    name=chat_name,
                    handle=chat_handle,
                )
                self.channel_map[resolved_id] = db_channel_id
                self.channel_map[chat_id] = db_channel_id
            except Exception as err:
                log_json(
                    "ERROR",
                    f"Failed to register channel {resolved_id} in DB: {err}",
                    correlation_id=correlation_id,
                    extra={"error": str(err)},
                )
                return

        # Skip messages if text is empty and no media
        if not getattr(msg, "text", None) and not getattr(msg, "media", None):
            return

        # Skip messages if channel is marked inactive
        if not self.db.is_channel_active(db_channel_id):
            msg_id = getattr(msg, "id", None)
            log_json(
                "DEBUG",
                f"Skipping message {msg_id} from inactive channel {db_channel_id}",
                correlation_id=correlation_id,
                extra={"channel_id": db_channel_id},
            )
            return

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
                log_json(
                    "INFO",
                    f"Ingested message {payload.telegram_message_id} from channel {db_channel_id}",
                    correlation_id=correlation_id,
                    extra={
                        "raw_post_id": raw_post_id,
                        "channel_id": db_channel_id,
                        "telegram_message_id": payload.telegram_message_id,
                        "status": "ingested",
                    },
                )
            else:
                log_json(
                    "INFO",
                    f"Ignored duplicate message {payload.telegram_message_id} (idempotent no-op)",
                    correlation_id=correlation_id,
                    extra={
                        "channel_id": db_channel_id,
                        "telegram_message_id": payload.telegram_message_id,
                        "status": "duplicate_skipped",
                    },
                )
        except Exception as err:
            log_json(
                "ERROR",
                f"Failed to ingest message {msg.id}: {err}",
                correlation_id=correlation_id,
                extra={"error": str(err), "telegram_message_id": getattr(msg, "id", None)},
            )

    async def start(self) -> None:
        """Starts the Telethon client, registers message handlers, and runs until stopped."""
        if self.client is None:
            await self.initialize_client()
        assert self.client is not None

        log_json("INFO", "Connecting to Telegram MTProto network...")

        if not self.config.telegram_session_string:
            # Interactive first-time login
            log_json(
                "INFO",
                "TELEGRAM_SESSION_STRING is empty. Initiating interactive Telegram login...",
            )
            await cast(Any, self.client.start())
            saved_session = self.client.session.save()
            sys.stdout.write("\n" + "=" * 80 + "\n")
            sys.stdout.write("TELEGRAM SESSION STRING GENERATED SUCCESSFULLY:\n\n")
            sys.stdout.write(f"{saved_session}\n\n")
            sys.stdout.write("Copy the string above and add it to your .env file as:\n")
            sys.stdout.write(f"TELEGRAM_SESSION_STRING={saved_session}\n")
            sys.stdout.write("=" * 80 + "\n\n")
            sys.stdout.flush()
        else:
            connected = False
            last_err = None
            max_retries = self.config.max_reconnect_retries
            for attempt in range(1, max_retries + 1):
                try:
                    log_json(
                        "INFO",
                        f"Connecting to Telegram network (attempt {attempt}/{max_retries})...",
                    )
                    await self.client.connect()
                    if not await self.client.is_user_authorized():
                        log_json(
                            "ERROR",
                            "Telegram session unauthorized. Valid session string required.",
                        )
                        raise PermissionError("Telegram session unauthorized")
                    connected = True
                    break
                except PermissionError:
                    raise
                except (ConnectionError, OSError, TimeoutError) as conn_err:
                    last_err = conn_err
                    log_json(
                        "WARN",
                        f"Telegram connection attempt {attempt} failed: {conn_err}. "
                        f"Retrying in {self.config.reconnect_delay_seconds}s...",
                        extra={"attempt": attempt, "error": str(conn_err)},
                    )
                    await asyncio.sleep(self.config.reconnect_delay_seconds)

            if not connected:
                log_json(
                    "ERROR",
                    f"Could not connect to Telegram after {max_retries} attempts: {last_err}",
                )
                raise ConnectionError(f"Failed to connect to Telegram: {last_err}")

        log_json("INFO", "Telegram client authorized successfully.")

        # Resolve channels
        resolved_entities = await self.resolve_channels()
        target_chats = resolved_entities if resolved_entities else None

        # Register event handler with both incoming and outgoing message events
        @self.client.on(events.NewMessage(chats=target_chats, incoming=None, outgoing=None))
        async def on_message_event(event: events.NewMessage.Event) -> None:
            await self.handle_new_message(event)

        log_json(
            "INFO",
            f"Telegram listener active and monitoring {len(resolved_entities)} channel(s).",
            extra={"monitored_channels": list(self.channel_map.keys())},
        )

        self.running = True
        try:
            await self.client.run_until_disconnected()
        finally:
            log_json("INFO", "Telegram listener disconnected.")

    async def stop(self) -> None:
        """Gracefully disconnects the Telethon client."""
        if self.client and self.client.is_connected():
            log_json("INFO", "Stopping Telegram client...")
            await self.client.disconnect()
        self.running = False


async def async_main() -> None:
    """Async entrypoint loading configuration and starting listener."""
    try:
        config = Config.from_env()
    except ValueError as err:
        if "TELEGRAM_SESSION_STRING" in str(err):
            try:
                config = Config.from_env(allow_empty_session=True)
            except Exception as e:
                log_json("ERROR", f"Configuration error: {e}")
                sys.exit(1)
        else:
            log_json("ERROR", f"Configuration error: {err}")
            sys.exit(1)

    db = Database(config.database_url)
    listener = TelegramListener(config, db)

    loop = asyncio.get_running_loop()

    def handle_shutdown_signal() -> None:
        log_json("INFO", "Received shutdown signal, initiating graceful stop...")
        asyncio.create_task(listener.stop())

    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, handle_shutdown_signal)
        except NotImplementedError:
            # On platforms where add_signal_handler is not implemented
            pass

    try:
        await listener.start()
    except Exception as err:
        log_json("ERROR", f"Listener stopped with error: {err}")
        sys.exit(1)


def main() -> None:
    """Synchronous entrypoint."""
    try:
        asyncio.run(async_main())
    except KeyboardInterrupt:
        log_json("INFO", "Process terminated by user.")


if __name__ == "__main__":
    main()
