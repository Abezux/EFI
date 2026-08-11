"""
Python-based migration runner for applying db/migrations/*.sql files.
Uses DATABASE_URL or APP_DATABASE_URL from .env.
"""

from __future__ import annotations

import os
from pathlib import Path

import psycopg

from services.listener.config import load_dotenv


def run_migrations() -> bool:
    load_dotenv()
    db_url = os.getenv("DATABASE_URL") or os.getenv("APP_DATABASE_URL")
    if not db_url:
        print("ERROR: Neither DATABASE_URL nor APP_DATABASE_URL is set in environment.")
        return False

    migrations_dir = Path(__file__).parent / "migrations"
    migration_files = sorted(migrations_dir.glob("*.sql"))

    if not migration_files:
        print(f"No migration files found in {migrations_dir}")
        return False

    print(f"Connecting to database: {db_url.split('@')[-1]}...")
    try:
        with psycopg.connect(db_url, autocommit=True) as conn:
            with conn.cursor() as cur:
                for sql_file in migration_files:
                    print(f"Applying migration: {sql_file.name}...")
                    sql_content = sql_file.read_text(encoding="utf-8")
                    cur.execute(sql_content)
                    print(f"  ✓ Applied {sql_file.name}")
        print("\nAll migrations applied successfully!")
        return True
    except Exception as e:
        print(f"\nMigration failed: {type(e).__name__}: {e}")
        return False


if __name__ == "__main__":
    import sys

    success = run_migrations()
    if not success:
        sys.exit(1)
