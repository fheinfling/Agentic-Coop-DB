#!/usr/bin/env bash
# scripts/gen-key.sh — convenience wrapper around `ai-coop-db-server -mint-key`.
#
# Usage:
#   ./scripts/gen-key.sh [workspace] [pg_role]
#
# Examples:
#   ./scripts/gen-key.sh                       # workspace=default, pg_role=dbadmin
#   ./scripts/gen-key.sh acme dbuser           # workspace=acme,    pg_role=dbuser
#   AICOOPDB_KEY_ENV=live ./scripts/gen-key.sh # tag the key as live instead of dev
#
# Resolution order for invoking the api binary:
#
#   1. AICOOPDB_API_CONTAINER env var (Coolify, k8s, any orchestrator
#      where the api runs as a named container) — `docker exec` into it
#   2. ai-coop-db-server binary on PATH (local builds or installed binaries)
#   3. local docker container literally named ai-coop-db-api
#      (the compose dev profile names it that way)
#
# All the actual work — random generation, argon2id hashing, INSERTing
# into aicoopdb.workspaces and aicoopdb.api_keys — happens inside the
# Go binary. This script is just a launcher.

set -euo pipefail

workspace="${1:-default}"
pg_role="${2:-dbadmin}"
env_tag="${AICOOPDB_KEY_ENV:-dev}"

args=(-mint-key
  -mint-workspace "${workspace}"
  -mint-role "${pg_role}"
  -mint-env "${env_tag}"
)

if [[ -n "${AICOOPDB_API_CONTAINER:-}" ]]; then
  exec docker exec "${AICOOPDB_API_CONTAINER}" /app/ai-coop-db-server "${args[@]}"
elif command -v ai-coop-db-server >/dev/null 2>&1; then
  exec ai-coop-db-server "${args[@]}"
elif command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -q '^ai-coop-db-api'; then
  exec docker exec ai-coop-db-api /app/ai-coop-db-server "${args[@]}"
fi

echo "could not invoke ai-coop-db-server -mint-key" >&2
echo "  set AICOOPDB_API_CONTAINER to the running api container name, or" >&2
echo "  install the ai-coop-db-server binary on PATH" >&2
exit 1
