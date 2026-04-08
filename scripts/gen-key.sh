#!/usr/bin/env bash
# scripts/gen-key.sh — mint an API key directly via psql.
#
# Usage:
#   ./scripts/gen-key.sh <workspace-name> [pg_role]
#
# Defaults:
#   pg_role = dbadmin   (so a fresh stack has a way in)
#
# Requires:
#   - the compose stack to be running
#   - psql in PATH
#   - DATABASE_URL env, or AICOOPDB_MIGRATIONS_DATABASE_URL, or the local
#     dev default (postgres://aicoopdb_owner@localhost:5432/aicoopdb?sslmode=disable)
#
# What it does:
#   1. ensures the named workspace exists, creating it if necessary
#   2. mints a fresh acd_dev_<id>_<secret> using openssl-generated entropy
#   3. inserts the row in api_keys with the argon2id hash via psql + pgcrypto
#   4. prints the full token to stdout exactly once
#
# This script is intended for the very first key on a fresh stack. After
# that, prefer `ai-coop-db key create` (Phase 6 CLI) which calls into the API.

set -euo pipefail

workspace="${1:-default}"
pg_role="${2:-dbadmin}"
env_tag="${AICOOPDB_KEY_ENV:-dev}"

URL="${DATABASE_URL:-${AICOOPDB_MIGRATIONS_DATABASE_URL:-postgres://aicoopdb_owner@localhost:5432/aicoopdb?sslmode=disable}}"

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not in PATH" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not in PATH" >&2
  exit 1
fi

# 12 random bytes -> 16 base64url chars (no padding) for the lookup id.
key_id="$(openssl rand 12 | base64 | tr '+/' '-_' | tr -d '=\n')"
# 24 random bytes -> 32 base64url chars (no padding) for the secret (~192 bits).
secret="$(openssl rand 24 | base64 | tr '+/' '-_' | tr -d '=\n')"
full="acd_${env_tag}_${key_id}_${secret}"

# Generate the argon2id hash inside Postgres using a small DO block. We rely
# on pgcrypto being available (it ships with Postgres) but the hashing
# itself is delegated to the api server's expected format. Since psql can't
# run argon2 directly, we use a sentinel that the gateway recognises and
# *re-hash on first use*. To keep the surface honest, however, we instead
# call out to the ai-coop-db-server binary if it is on the host PATH.
hash=""
if command -v ai-coop-db-server >/dev/null 2>&1; then
  hash="$(ai-coop-db-server -hash-secret "${secret}")"
elif command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -q '^ai-coop-db-api'; then
  hash="$(docker exec ai-coop-db-api /app/ai-coop-db-server -hash-secret "${secret}")"
fi
if [[ -z "${hash}" ]]; then
  echo "could not invoke ai-coop-db-server -hash-secret; install the binary or run inside the api container" >&2
  exit 1
fi

ws_id="$(uuidgen | tr 'A-Z' 'a-z')"
key_pk="$(uuidgen | tr 'A-Z' 'a-z')"

psql "${URL}" <<SQL
SET client_min_messages = WARNING;

INSERT INTO workspaces (id, name)
VALUES ('${ws_id}', '${workspace}')
ON CONFLICT (name) DO NOTHING;

WITH ws AS (
    SELECT id FROM workspaces WHERE name = '${workspace}' LIMIT 1
)
INSERT INTO api_keys (id, workspace_id, key_id, secret_hash, env, pg_role, name)
SELECT '${key_pk}', ws.id, '${key_id}', '${hash}', '${env_tag}', '${pg_role}', 'gen-key.sh'
FROM ws;
SQL

echo
echo "API key minted (shown only once):"
echo "    ${full}"
echo
echo "Use it with:"
echo "    curl -H 'Authorization: Bearer ${full}' http://localhost:8080/v1/me"
