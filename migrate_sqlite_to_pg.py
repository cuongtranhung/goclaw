"""
Migrate GoClaw standalone (SQLite/file) data to managed (PostgreSQL) mode.
Migrates:
  1. user_agent_profiles from agents.db
  2. Session conversation history from JSON files
"""

import sqlite3, json, os, re, psycopg2
from datetime import datetime

# Config
AGENTS_DB   = r"C:\Users\Administrator\.goclaw\data\agents.db"
SESSIONS_DIR = r"C:\Users\Administrator\.goclaw\sessions"
PG_DSN      = {"host": "localhost", "port": 5432, "dbname": "goclaw", "user": "postgres", "password": "@abcd1234"}
NEW_AGENT_ID = "019ca8bc-29d8-73ab-8431-268a82385593"   # default agent in PG
NEW_CHANNEL  = "telegram-main"

pg = psycopg2.connect(**PG_DSN)
cur = pg.cursor()

# ── 1. Migrate user_agent_profiles ──────────────────────────────────────────
print("=== Migrating user_agent_profiles ===")
sq = sqlite3.connect(AGENTS_DB)
sqcur = sq.cursor()
sqcur.execute("SELECT user_id, workspace, first_seen_at, last_seen_at FROM user_profiles")
for row in sqcur.fetchall():
    user_id, workspace, first_seen, last_seen = row
    print(f"  User: {user_id}, first={first_seen}, last={last_seen}")
    cur.execute("""
        INSERT INTO user_agent_profiles (agent_id, user_id, workspace, first_seen_at, last_seen_at)
        VALUES (%s, %s, %s, %s, %s)
        ON CONFLICT (agent_id, user_id) DO UPDATE
            SET last_seen_at = EXCLUDED.last_seen_at,
                workspace = EXCLUDED.workspace
    """, (NEW_AGENT_ID, user_id, workspace, first_seen, last_seen))
sq.close()
print(f"  Done.")

# ── 2. Migrate sessions ──────────────────────────────────────────────────────
print("\n=== Migrating sessions ===")

# Map old file names to session keys
# Pattern: agent_{agent_key}_{channel}_{peer_kind}_{user_id}.json
# New session_key: agent:{agent_key}:{channel_name}:{peer_kind}:{user_id}

channel_map = {
    "telegram": NEW_CHANNEL,
}

for fname in os.listdir(SESSIONS_DIR):
    if not fname.endswith(".json"):
        continue

    fpath = os.path.join(SESSIONS_DIR, fname)
    try:
        with open(fpath, encoding="utf-8") as f:
            data = json.load(f)
    except Exception as e:
        print(f"  Skip {fname}: {e}")
        continue

    messages = data.get("messages", [])
    if not messages:
        print(f"  Skip {fname}: no messages")
        continue

    # Parse filename: agent_default_telegram_direct_6539583633.json
    m = re.match(r"agent_(\w+)_(\w+)_(\w+)_(.+)\.json", fname)
    if not m:
        print(f"  Skip {fname}: unrecognized format")
        continue

    agent_key, channel_type, peer_kind, user_id = m.groups()

    # Skip non-telegram or cron sessions
    if channel_type not in channel_map:
        print(f"  Skip {fname}: channel type '{channel_type}' not mapped")
        continue

    channel_name = channel_map[channel_type]
    session_key = f"agent:{agent_key}:{channel_name}:{peer_kind}:{user_id}"

    print(f"  {fname} -> {session_key} ({len(messages)} messages)")

    summary  = data.get("summary", "")
    model    = data.get("model", "")
    provider = data.get("provider", "gemini")
    input_tokens  = data.get("input_tokens", 0)
    output_tokens = data.get("output_tokens", 0)

    cur.execute("""
        INSERT INTO sessions (
            session_key, agent_id, user_id, messages, summary,
            model, provider, channel, input_tokens, output_tokens
        ) VALUES (%s, %s, %s, %s::jsonb, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (session_key) DO UPDATE
            SET messages      = EXCLUDED.messages,
                summary       = EXCLUDED.summary,
                input_tokens  = EXCLUDED.input_tokens,
                output_tokens = EXCLUDED.output_tokens,
                updated_at    = NOW()
    """, (
        session_key, NEW_AGENT_ID, user_id,
        json.dumps(messages), summary,
        model, provider, channel_name,
        input_tokens, output_tokens
    ))
    print(f"    Inserted/updated OK")

pg.commit()
pg.close()
print("\n=== Migration complete! ===")
