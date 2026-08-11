"""
Fixture ingestion and normalization test using tests/fixtures/sample_telegram_posts.json
"""

import json
from pathlib import Path

from services.listener.ingest import normalize_raw_message


def load_fixtures():
    base = Path(__file__).resolve().parents[3]
    candidates = [
        Path("tests/fixtures/sample_telegram_posts.json"),
        base / "tests" / "fixtures" / "sample_telegram_posts.json",
    ]
    for p in candidates:
        if p.is_file():
            with open(p, encoding="utf-8") as f:
                return json.load(f)
    raise FileNotFoundError("Could not locate sample_telegram_posts.json fixture")


def test_normalize_all_sample_fixtures():
    fixtures = load_fixtures()
    assert len(fixtures) >= 3

    for item in fixtures:
        payload = normalize_raw_message(
            channel_id=1,
            message_id=item["message_id"],
            raw_text=item["raw_text"],
            posted_at=item["posted_at"],
            raw_entities=item.get("raw_entities", []),
            media_refs=item.get("media_refs", []),
        )

        assert payload.channel_id == 1
        assert payload.telegram_message_id == item["message_id"]
        assert payload.raw_text == item["raw_text"]
        assert payload.processing_status == "ingested"
        assert len(payload.raw_text) > 0
        assert payload.posted_at is not None
        assert isinstance(payload.raw_entities, list)
        assert isinstance(payload.media_refs, list)


def test_fixture_idempotency_simulation():
    fixtures = load_fixtures()

    # Simulate database insertion set with (channel_id, telegram_message_id) constraint
    stored_posts = {}
    insert_counts = 0

    for _ in range(2):  # Replay all fixtures twice
        for item in fixtures:
            payload = normalize_raw_message(
                channel_id=item["channel_id"],
                message_id=item["message_id"],
                raw_text=item["raw_text"],
                posted_at=item["posted_at"],
                raw_entities=item.get("raw_entities", []),
                media_refs=item.get("media_refs", []),
            )

            key = (payload.channel_id, payload.telegram_message_id)
            if key not in stored_posts:
                stored_posts[key] = payload
                insert_counts += 1

    # Exactly len(fixtures) rows should exist, zero duplicates
    assert len(stored_posts) == len(fixtures)
    assert insert_counts == len(fixtures)
