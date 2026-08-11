#!/usr/bin/env python3
"""
Validates tests/fixtures/sample_telegram_posts.json fixture structure and content.
"""
import json
import os
import sys
from datetime import datetime

def validate_fixtures():
    fixture_path = os.path.join(os.path.dirname(__file__), "fixtures", "sample_telegram_posts.json")
    if not os.path.exists(fixture_path):
        print(f"FAIL: Fixture not found at {fixture_path}", file=sys.stderr)
        return 1

    with open(fixture_path, "r", encoding="utf-8") as f:
        try:
            posts = json.load(f)
        except json.JSONDecodeError as e:
            print(f"FAIL: Invalid JSON in fixture: {e}", file=sys.stderr)
            return 1

    if not isinstance(posts, list) or len(posts) < 3:
        print(f"FAIL: Expected list with >= 3 posts, got {len(posts) if isinstance(posts, list) else type(posts)}", file=sys.stderr)
        return 1

    fuel_posts = 0
    seen_keys = set()

    for idx, post in enumerate(posts):
        for req_field in ["channel_id", "message_id", "raw_text", "posted_at"]:
            if req_field not in post or post[req_field] is None:
                print(f"FAIL: Post [{idx}] missing required field '{req_field}'", file=sys.stderr)
                return 1

        if not str(post["raw_text"]).strip():
            print(f"FAIL: Post [{idx}] has empty raw_text", file=sys.stderr)
            return 1

        # Check timestamp
        try:
            datetime.fromisoformat(post["posted_at"].replace("Z", "+00:00"))
        except ValueError as e:
            print(f"FAIL: Post [{idx}] invalid ISO/RFC3339 timestamp '{post['posted_at']}': {e}", file=sys.stderr)
            return 1

        key = (post["channel_id"], post["message_id"])
        if key in seen_keys:
            print(f"FAIL: Duplicate (channel_id, message_id) found: {key}", file=sys.stderr)
            return 1
        seen_keys.add(key)

        text = post["raw_text"]
        if "101" in text and ("ነዳጅ" in text or "ቤንዚን" in text):
            fuel_posts += 1

    if fuel_posts < 2:
        print(f"FAIL: Expected at least 2 near-duplicate posts covering the fuel price event, found {fuel_posts}", file=sys.stderr)
        return 1

    print(f"SUCCESS: Validated {len(posts)} posts in {fixture_path} (including {fuel_posts} near-duplicate event posts).")
    return 0

if __name__ == "__main__":
    sys.exit(validate_fixtures())
