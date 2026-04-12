# MCP Server

The MCP (Model Context Protocol) server lets AI agents connect to Agentic Coop DB
through any MCP-compatible client — Claude Desktop, Claude Code, Cursor, or
custom agent frameworks.

## Architecture

The MCP server is a **standalone binary** that acts as an HTTP client to the
gateway. Every tool call results in an authenticated HTTP request that
traverses the full middleware chain: auth, rate limiting, tenant isolation,
SQL validation, and audit logging. The MCP binary never accesses the database
directly.

```
agent ──MCP/stdio──► agentic-coop-db-mcp ──HTTPS──► gateway ──pgx──► postgres
```

## Install

### Pre-built binary (recommended)

Download the binary for your platform from the
[latest release](https://github.com/fheinfling/agentic-coop-db/releases/latest),
extract it, and place it on your `PATH`:

```bash
# macOS (Apple Silicon)
curl -fsSL https://github.com/fheinfling/agentic-coop-db/releases/latest/download/agentic-coop-db-mcp-darwin-arm64.tar.gz \
  | tar xz && chmod +x agentic-coop-db-mcp && sudo mv agentic-coop-db-mcp /usr/local/bin/
```

| Platform | Archive |
|----------|---------|
| macOS (Apple Silicon) | `agentic-coop-db-mcp-darwin-arm64.tar.gz` |
| macOS (Intel) | `agentic-coop-db-mcp-darwin-amd64.tar.gz` |
| Linux (x86_64) | `agentic-coop-db-mcp-linux-amd64.tar.gz` |
| Linux (ARM64) | `agentic-coop-db-mcp-linux-arm64.tar.gz` |
| Windows (x86_64) | `agentic-coop-db-mcp-windows-amd64.zip` |

### go install

Requires Go 1.26+:

```bash
go install github.com/fheinfling/agentic-coop-db/cmd/mcp@latest
```

The binary is installed as `mcp` in `$GOPATH/bin`. Rename or symlink if desired:

```bash
mv "$(go env GOPATH)/bin/mcp" "$(go env GOPATH)/bin/agentic-coop-db-mcp"
```

### Docker

The container image includes the binary at `/app/agentic-coop-db-mcp`:

```bash
docker run -i --rm \
  -e AGENTCOOPDB_GATEWAY_URL=https://db.example.com \
  -e AGENTCOOPDB_API_KEY=acd_live_<id>_<secret> \
  ghcr.io/fheinfling/agentic-coop-db-server:latest /app/agentic-coop-db-mcp
```

### Build from source

```bash
make build-mcp          # produces bin/agentic-coop-db-mcp
```

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `AGENTCOOPDB_GATEWAY_URL` | Yes | Base URL of the gateway (e.g. `https://db.example.com`) |
| `AGENTCOOPDB_API_KEY` | Yes* | API key (`acd_<env>_<id>_<secret>`) |
| `AGENTCOOPDB_API_KEY_FILE` | No | File path containing the API key (docker secret pattern; used when `AGENTCOOPDB_API_KEY` is not set) |

*Either `AGENTCOOPDB_API_KEY` or `AGENTCOOPDB_API_KEY_FILE` is required.

## Client integration

All examples below assume `agentic-coop-db-mcp` is on your `PATH`. If you
placed it elsewhere, use the full path instead.

### Claude Code

One command:

```bash
claude mcp add agentic-coop-db \
  -e AGENTCOOPDB_GATEWAY_URL=https://db.example.com \
  -e AGENTCOOPDB_API_KEY=acd_live_<id>_<secret> \
  -- agentic-coop-db-mcp
```

Or add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "agentic-coop-db": {
      "command": "agentic-coop-db-mcp",
      "env": {
        "AGENTCOOPDB_GATEWAY_URL": "https://db.example.com",
        "AGENTCOOPDB_API_KEY": "acd_live_<id>_<secret>"
      }
    }
  }
}
```

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)
or `%APPDATA%/Claude/claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "agentic-coop-db": {
      "command": "agentic-coop-db-mcp",
      "env": {
        "AGENTCOOPDB_GATEWAY_URL": "https://db.example.com",
        "AGENTCOOPDB_API_KEY": "acd_live_<id>_<secret>"
      }
    }
  }
}
```

### Cursor

Add to Settings > MCP Servers, or to `.cursor/mcp.json` in your project:

```json
{
  "mcpServers": {
    "agentic-coop-db": {
      "command": "agentic-coop-db-mcp",
      "env": {
        "AGENTCOOPDB_GATEWAY_URL": "https://db.example.com",
        "AGENTCOOPDB_API_KEY": "acd_live_<id>_<secret>"
      }
    }
  }
}
```

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "agentic-coop-db": {
      "command": "agentic-coop-db-mcp",
      "env": {
        "AGENTCOOPDB_GATEWAY_URL": "https://db.example.com",
        "AGENTCOOPDB_API_KEY": "acd_live_<id>_<secret>"
      }
    }
  }
}
```

### Verify

After configuring any client, ask your agent to _"use the whoami tool"_. It
should return your workspace, role, and environment.

## Available tools

### sql_execute

Execute a parameterized SQL statement.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sql` | string | Yes | SQL with `$N` placeholders |
| `params` | array | No | Parameter values matching placeholders |
| `idempotency_key` | string | No | Forwarded as `Idempotency-Key` header |

**Output:** `{command, columns, rows, rows_affected, duration_ms}`

### rpc_call

Call a registered RPC procedure.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `procedure` | string | Yes | Procedure name |
| `args` | object | No | Procedure arguments |

**Output:** Procedure result (JSON).

### list_tables

List all user tables in the public schema with approximate row counts.
No input required.

### describe_table

Show column definitions for a table.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `table` | string | Yes | Table name (lowercase alphanumeric/underscore) |

**Output:** Column names, types, nullability, defaults.

### vector_search

Top-k cosine similarity search on a pgvector column.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `table` | string | Yes | Table with vector column |
| `vector_column` | string | Yes | Vector column name |
| `query_embedding` | array | Yes | Query vector (array of floats) |
| `k` | number | No | Number of results (default 5) |

### vector_upsert

Insert or update embedding rows (INSERT ... ON CONFLICT).

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `table` | string | Yes | Table name |
| `id_column` | string | Yes | Primary key column |
| `vector_column` | string | Yes | Vector column |
| `rows` | array | Yes | Array of `{id, metadata, vector}` objects |

### whoami

Show current workspace, role, and environment.

### health

Check gateway health and readiness.

## Security

The MCP server is a pure HTTP client — it has the same trust level as the
Python SDK or curl. All security enforcement happens at the gateway:

- **Authentication:** API key verified with Argon2id
- **Authorization:** `SET LOCAL ROLE` per transaction
- **Tenant isolation:** RLS via `app.workspace_id` GUC
- **SQL validation:** Parser-backed, single-statement, parameterized
- **Audit:** Every call logged

**Recommendations:**

- Use a `dbuser` key for MCP, not `dbadmin`, unless you need DDL
- Use `AGENTCOOPDB_API_KEY_FILE` in production instead of env vars
- Rotate keys regularly via `/v1/auth/keys/rotate`

## Troubleshooting

**"AGENTCOOPDB_GATEWAY_URL is required"** — Set the environment variable in
your MCP client config.

**401 Unauthorized** — The API key is invalid or revoked. Mint a new one with
`./scripts/gen-key.sh <workspace> <role>`.

**Connection refused** — The gateway is not running or not reachable from the
MCP binary. Verify with `curl <gateway_url>/healthz`.

**Timeout errors** — The gateway's statement timeout (default 5s) may be too
short for complex queries. Contact your admin to adjust
`AGENTCOOPDB_STATEMENT_TIMEOUT`.
