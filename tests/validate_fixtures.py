"""
Validates tests/fixtures/sample_telegram_posts.json fixture structure and content.
"""

import json
import os
import sys
from datetime import datetime


def validate_fixtures():
    fixture_path = os.path.join(
        os.path.dirname(__file__), "fixtures", "sample_telegram_posts.json"
    )
    if not os.path.exists(fixture_path):
        print(f"FAIL: Fixture file not found at {fixture_path}", file=sys.stderr)
        return 1

    with open(fixture_path, encoding="utf-8") as f:
        try:
            posts = json.load(f)
        except json.JSONDecodeError as e:
            print(f"FAIL: Invalid JSON in {fixture_path}: {e}", file=sys.stderr)
            return 1

    if not isinstance(posts, list) or len(posts) < 3:
        err_val = len(posts) if isinstance(posts, list) else type(posts)
        print(f"FAIL: Expected list with >= 3 posts, got {err_val}", file=sys.stderr)
        return 1

    required_fields = {
        "channel_id",
        "telegram_message_id",
        "raw_text",
        "posted_at",
        "language",
    }
    fuel_posts = 0

    for idx, post in enumerate(posts):
        if not isinstance(post, dict):
            print(f"FAIL: Post [{idx}] is not a JSON object", file=sys.stderr)
            return 1

        missing = required_fields - set(post.keys())
        if missing:
            print(f"FAIL: Post [{idx}] missing fields: {missing}", file=sys.stderr)
            return 1

        try:
            datetime.fromisoformat(post["posted_at"].replace("Z", "+00:00"))
        except ValueError as e:
            print(
                f"FAIL: Post [{idx}] invalid ISO timestamp '{post['posted_at']}': {e}",
                file=sys.stderr,
            )
            return 1

        if not post["raw_text"] or not isinstance(post["raw_text"], str):
            print(f"FAIL: Post [{idx}] empty raw_text", file=sys.stderr)
            return 1

        text = post["raw_text"].lower()
        if "fuel" in text or "ነዳጅ" in text:
            fuel_posts += 1

    if fuel_posts < 2:
        print(
            f"FAIL: Expected >= 2 near-duplicate posts for fuel price, found {fuel_posts}",
            file=sys.stderr,
        )
        return 1

    print(
        f"SUCCESS: Validated {len(posts)} posts in {fixture_path} "
        f"(including {fuel_posts} near-duplicate event posts)."
    )
    return 0


if __name__ == "__main__":
    sys.exit(validate_fixtures())
