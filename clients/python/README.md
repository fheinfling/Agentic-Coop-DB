# aicoopdb (Python client)

A thin Python client for [AI Coop DB](https://github.com/fheinfling/ai-coop-db), the
auth gateway for shared PostgreSQL.

```python
from aicoopdb import connect

db = connect("https://db.example.com", api_key="acd_live_...")
db.execute(
    "INSERT INTO notes(id, body) VALUES ($1, $2)",
    ["b9c3...", "hi"],
)
rows = db.select("SELECT * FROM notes WHERE owner = $1", ["alice"])
```

## Install

```bash
pip install ai-coop-db
```

## CLI

```bash
ai-coop-db init                  # interactive onboarding wizard
ai-coop-db me
ai-coop-db sql "SELECT 1"
ai-coop-db queue flush
ai-coop-db doctor
```

See the [main repo](https://github.com/fheinfling/ai-coop-db) for the full docs.
