import sqlite3, json, os

# Inspect memory.db
memory_db = r"C:\Users\Administrator\.goclaw\workspace\memory.db"
agents_db = r"C:\Users\Administrator\.goclaw\data\agents.db"

for db_path in [memory_db, agents_db]:
    if not os.path.exists(db_path):
        print(f"NOT FOUND: {db_path}")
        continue
    print(f"\n=== {db_path} ===")
    conn = sqlite3.connect(db_path)
    cur = conn.cursor()
    cur.execute("SELECT name FROM sqlite_master WHERE type='table'")
    tables = cur.fetchall()
    for t in tables:
        name = t[0]
        cur.execute(f'SELECT COUNT(*) FROM "{name}"')
        count = cur.fetchone()[0]
        print(f"  {name}: {count} rows")
        if count > 0 and count <= 5:
            cur.execute(f'SELECT * FROM "{name}" LIMIT 3')
            rows = cur.fetchall()
            for r in rows:
                print(f"    {str(r)[:120]}")
    conn.close()
