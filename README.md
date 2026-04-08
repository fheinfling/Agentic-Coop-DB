# Agentic Coop DB — auth gateway for shared PostgreSQL

You can't expose Postgres on the public internet. Agentic Coop DB lets your agents talk
to a remote Postgres + pgvector instance using nothing but an HTTPS URL and an
API key. CRUD SQL goes through unchanged — no new query language, no ORM
lock-in.

**Status:** v0.1 — single-node, container-first, ARM64-friendly.
**License:** Apache-2.0.

---

## What it is

A thin auth gateway in front of PostgreSQL 16 + pgvector that does four jobs:

1. **Authenticate** the caller via a workspace-scoped API key.
2. **Authorize** by attaching a Postgres role to the request transaction
   (`SET LOCAL ROLE`) — Postgres itself decides what the key can run.
3. **Forward** the SQL with parameterized binding and a statement timeout.
4. **Audit** every call.

If you can write SQL, you can use it. `SELECT`, `INSERT`, `UPDATE`, `DELETE`,
`CREATE TABLE`, `CREATE USER`, `GRANT`, pgvector ops — all forwarded.

## What it is not

- Not a new query language, ORM, or schema migrator.
- Not realtime / websocket subscriptions (see `docs/features/0011-*`).
- Not object storage (see `docs/features/0012-*`).
- Not a serverless function runtime.
- Not multi-region or HA — single-node only in v1.
- Not a web UI — CLI + curl + your own app.
- Not SSO — API keys only.

## 30-second quickstart

```bash
git clone https://github.com/fheinfling/agentic-coop-db.git
cd agentic-coop-db
make up-local        # builds image, starts postgres + api, runs migrations
./scripts/gen-key.sh default dbadmin
# prints: acd_dev_<id>_<secret>   <-- copy this once, it is shown only here
```

Then from any app:

**Python**
```python
from agentcoopdb import connect

db = connect("http://localhost:8080", api_key="acd_dev_...")
db.execute(
    "CREATE TABLE IF NOT EXISTS notes (id uuid PRIMARY KEY, body text)"
)
db.execute(
    "INSERT INTO notes(id, body) VALUES ($1, $2)",
    [uuid7(), "hi"],
)
rows = db.select("SELECT * FROM notes WHERE body = $1", ["hi"])
```

**curl**
```bash
curl -X POST http://localhost:8080/v1/sql/execute \
  -H "Authorization: Bearer acd_dev_..." \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT * FROM notes WHERE body = $1", "params": ["hi"]}'
```

**JavaScript** (no SDK needed)
```js
await fetch("http://localhost:8080/v1/sql/execute", {
  method: "POST",
  headers: {
    Authorization: "Bearer acd_dev_...",
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    sql: "INSERT INTO notes(id, body) VALUES ($1, $2)",
    params: [id, "hi"],
  }),
});
```

## Connecting an AI agent

The gateway is a natural fit for AI agent workloads:

- **HTTP-only** — no native Postgres driver required in the agent runtime.
- **API-key auth** — easy to inject via environment variable or secrets manager.
- **Parameterized SQL** — the gateway rejects non-parameterized calls at parse
  time, which means LLM-generated SQL cannot embed literal values (a common
  injection vector).
- **pgvector** — store and search embeddings alongside structured data.
- **Idempotency keys** — retryable writes even over flaky networks.

### Setup

**1. Generate a key for the agent** (least-privilege: `dbuser`, not `dbadmin`):

```bash
./scripts/gen-key.sh <workspace> dbuser
# prints: acd_dev_<id>_<secret>   ← store in your agent's secrets manager
```

**2. Install the SDK:**

```bash
pip install agentic-coop-db
```

**3. Connect:**

```python
from agentcoopdb import connect

db = connect("https://db.example.com", api_key="acd_live_...")
me = db.me()   # verify connectivity: {workspace, role, server}
```

### Schema initialisation

The gateway enforces **one statement per HTTP call** (this prevents
multi-statement injection). When replaying a schema file, send each statement
as a separate `db.execute()` call. Use `CREATE … IF NOT EXISTS` / `DO $$ … $$`
guards so the sequence is fully idempotent — re-running from any point is safe.

### Multi-write atomicity

For writes that must land together, use the CTE-wrapped transaction helper:

```python
with db.transaction() as tx:
    tx.execute("INSERT INTO events (id, type) VALUES ($1, $2)", [eid, "start"])
    tx.execute("UPDATE jobs SET status=$1 WHERE id=$2", ["running", jid])
# Both writes execute as a single CTE-wrapped statement
```

### Vector / RAG

```python
# Store embeddings
db.vector_upsert("documents", [
    {"id": doc_id, "metadata": {"title": "…"}, "vector": embedding},
])

# Nearest-neighbour search
results = db.vector_search("documents", query_embedding, k=5)
```

---

## How it stays safe

PostgreSQL is the source of truth for what each key can do. The gateway only
enforces the minimum that Postgres cannot enforce by itself:

- **Parameterization is mandatory.** The body is `{sql, params}`. The validator
  parses the SQL and counts `$N` placeholders; mismatch = HTTP 400.
- **Single statement only.** Stacked-statement injection is rejected at parse
  time.
- **Statement size cap** (default 256 KiB, tunable via `AGENTCOOPDB_MAX_STATEMENT_BYTES`)
  and **parameter count cap** (default 1 000, tunable via `AGENTCOOPDB_MAX_STATEMENT_PARAMS`).
- **`SET LOCAL ROLE <key.role>`** before forwarding. The pool's login role is
  `agentcoopdb_gateway`, a low-privilege role with no privileges of its own — it
  is only a *member* of the role each key is bound to.
- **Built-in roles:** `dbadmin` (DDL/DCL, owner of `public`, `BYPASSRLS`,
  not superuser) and `dbuser` (CRUD, `NOBYPASSRLS`).
- **Filesystem escape functions** (`pg_read_file`, `lo_import`,
  `dblink_*`, `COPY ... FROM PROGRAM`) are revoked at the database level —
  even an admin key cannot read files off the host.
- **RLS** is the recommended pattern for tenant tables and `dbuser` cannot
  bypass it.
- **TLS** is mandatory in any non-localhost deployment. The `cloud` profile
  uses Caddy auto-TLS.

Full threat model: [`docs/security.md`](docs/security.md).

## Run it on…

| Profile      | File                              | Use case                                  |
|--------------|-----------------------------------|-------------------------------------------|
| `local`      | `deploy/compose.local.yml`        | Dev box, integration tests                |
| `pi-lite`    | `deploy/compose.pi-lite.yml`      | Raspberry Pi 4/5, low-mem ARM64           |
| `cloud`      | `deploy/compose.cloud.yml`        | Hetzner / DO / AWS / bare metal + Caddy   |
| `swarm`      | `deploy/stack.swarm.yml`          | Docker Swarm with external secrets        |

```bash
make up-local        # localhost:8080, no TLS
make up-pi           # ARM64-tuned postgres, low mem
make up-cloud        # Caddy auto-TLS, backups, prometheus
```

See [`docs/deploy-cloud.md`](docs/deploy-cloud.md) for worked examples on
Hetzner, DigitalOcean, AWS Lightsail, and bare metal.

## API surface

| Method | Path                       | Purpose                                 |
|--------|----------------------------|-----------------------------------------|
| `POST` | `/v1/sql/execute`          | Forward parameterized SQL               |
| `POST` | `/v1/rpc/call`             | Call a registered RPC (optional)        |
| `POST` | `/v1/auth/keys/rotate`     | Rotate the calling key                  |
| `GET`  | `/v1/me`                   | `{workspace, role, server_version}`     |
| `GET`  | `/healthz`                 | Liveness                                |
| `GET`  | `/readyz`                  | Ready (DB + migrations)                 |
| `GET`  | `/metrics`                 | Prometheus                              |

Full reference: [`docs/api.md`](docs/api.md).

## Repository layout

- `cmd/server` — API server entrypoint
- `cmd/migrate` — standalone migrator (also embedded in the server)
- `internal/` — implementation (clean layered architecture)
- `migrations/` — numbered SQL migrations (golang-migrate)
- `clients/python` — Python SDK + CLI (`pip install agentic-coop-db`)
- `deploy/` — compose files for local, pi-lite, cloud, swarm
- `docs/` — architecture, deployment, security, ADRs, feature roadmap
- `test/integration` — testcontainers-go full-stack tests
- `test/security` — cross-tenant + SQL bypass tests
- `scripts/` — dev helpers and `verify-acs.sh`

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — clean layers and how requests flow
- [`docs/api.md`](docs/api.md) — endpoint reference + curl examples
- [`docs/security.md`](docs/security.md) — threat model + reporting
- [`docs/rls.md`](docs/rls.md) — multi-tenant pattern with row-level security
- [`docs/rpc-authoring.md`](docs/rpc-authoring.md) — when to register an RPC
- [`docs/deploy-local.md`](docs/deploy-local.md) — local dev
- [`docs/deploy-pi-lite.md`](docs/deploy-pi-lite.md) — Raspberry Pi
- [`docs/deploy-cloud.md`](docs/deploy-cloud.md) — single-node cloud
- [`docs/faq.md`](docs/faq.md) — frequently asked questions
- [`docs/adr/`](docs/adr/) — architectural decision records
- [`docs/features/`](docs/features/) — roadmap (one file per future feature)

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). All commits must be signed off
(`git commit -s`) under the Developer Certificate of Origin.

Good first issues are tracked under the
[`good-first-issue`](https://github.com/fheinfling/agentic-coop-db/labels/good-first-issue)
label and as Markdown files in [`docs/features/`](docs/features/).

## Security

Report vulnerabilities privately via GitHub Security Advisories — see
[`SECURITY.md`](SECURITY.md). Critical fixes get a CVE and a patch release
within 7 days of confirmed report.

## License

Apache-2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
