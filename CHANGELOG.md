# Changelog

All notable changes to AI Coop DB are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **Project renamed** from `AIColDB` to `AI Coop DB`. Repository
  `github.com/fheinfling/ai-coop-db`, Go module
  `github.com/fheinfling/ai-coop-db`, Python distribution `ai-coop-db`,
  Python import `aicoopdb`, Postgres roles `aicoopdb_owner` /
  `aicoopdb_gateway`, env var prefix `AICOOPDB_*`, API key prefix `acd_`.

### Security
- **Control-plane tables split into a dedicated `aicoopdb` schema**
  (migration `0007_split_control_plane_schema`). Previously, `dbadmin`
  API keys could `DROP TABLE api_keys` because the control-plane lived in
  `public` (which `dbadmin` owns). They now live in `aicoopdb`, which
  only `aicoopdb_gateway` has CRUD on. `dbadmin` keys can still do any
  DDL/DCL on user data in `public` — the privilege boundary is enforced
  at the schema level, not by restricting `dbadmin`.
- **C3** — `internal/sql.Validator` now counts `$N` placeholders by
  walking `pg_query.Scan` tokens instead of running `pg_query.Normalize`,
  which previously inflated the count for any query that contained a
  literal and made nearly every real-world request fail with
  `params_mismatch`.
- **C4** — `POST /v1/auth/keys` now rejects requests where
  `workspace_id` differs from the calling key's workspace. A compromised
  `dbadmin` key for tenant A can no longer mint keys for tenant B.
- **C5** — proper password handling. New `AICOOPDB_GATEWAY_PASSWORD` and
  `AICOOPDB_OWNER_PASSWORD` env vars (plus the docker `_FILE` variants);
  the server runs `ALTER ROLE aicoopdb_gateway WITH PASSWORD` after
  migrations and injects the password into the pool config. The
  `compose.cloud.yml` profile mounts both as docker secrets. The local /
  pi-lite profiles set `POSTGRES_HOST_AUTH_METHOD=trust` so the dev
  experience stays passwordless.
- **H1** — `auth.VerifyCache.RevokeByDBID` evicts every cached entry for
  a key DB id. The HTTP rotate handler calls it after a successful
  rotation, so revoked tokens stop working immediately instead of
  surviving up to the LRU TTL (5 minutes).
- **H2** — the auth middleware runs argon2id against a precomputed
  dummy hash on `ErrKeyNotFound` so the response time of "wrong key_id"
  is indistinguishable from "right key_id, wrong secret". Closes the
  timing oracle that allowed external enumeration of valid key ids.
- **H3** — `rpc.HashRequest` now hashes the raw request body bytes
  (sha256 of `method | 0 | path | 0 | body`) instead of re-encoding the
  parsed args, which was non-deterministic for nested JSON objects.
- **H4** — `POST /v1/sql/execute` now honours `Idempotency-Key`,
  sharing the same state machine as `/v1/rpc/call`. The Python SDK's
  offline retry queue can finally claim "exactly once" with confidence.
- **H5** — the Python SDK auto-generates an `Idempotency-Key` header
  for every `_post` call that does not already have one, so retries on
  transport-level errors no longer risk duplicate writes.

## [0.1.0] — 2026-04-08

Initial public release. Single-node, container-first auth gateway in front
of PostgreSQL 16 + pgvector.

### Added

- **Auth gateway** with workspace-scoped API keys (`acd_<env>_<id>_<secret>`),
  argon2id at rest, in-memory LRU verify cache, key rotation with overlap.
- **Parameterised SQL forwarding** at `POST /v1/sql/execute`. Validator
  enforces single-statement, parse-success, placeholder-count match,
  size + param caps. No statement-type allowlist — Postgres role grants
  decide what each key can run.
- **Multi-tenant isolation** via `SET LOCAL ROLE` + RLS policies keyed
  on `app.workspace_id`. Migration linter (`scripts/lint-migrations`) is
  a CI gate that fails any tenant table without the policy.
- **Built-in roles** `dbadmin` (DDL/DCL, owner of `public`, `BYPASSRLS`)
  and `dbuser` (CRUD, `NOBYPASSRLS`). Custom roles supported via
  `ai-coop-db key create --role`.
- **pgvector** enabled by migration 0005, with `internal/vector` helpers
  and `db.vector_upsert` / `db.vector_search` in the Python SDK.
- **Optional RPC layer** at `POST /v1/rpc/call` with JSON Schema arg
  validation, server-side idempotency-key replay/conflict, and
  `sql/rpc/upsert_document.sql` as a worked example.
- **Audit log** (`audit_logs` table) with hashed SQL/params; full capture
  via `AICOOPDB_AUDIT_INCLUDE_SQL=true`.
- **Rate limiting** per key via `golang.org/x/time/rate` (60 req/s, burst
  120, configurable). Returns HTTP 429 with `Retry-After`.
- **Postgres-side hardening**: filesystem escape functions
  (`pg_read_file`, `lo_import`, `COPY ... FROM PROGRAM`, etc) revoked at
  the database level by migration 0004.
- **Container baseline**: multi-stage distroless ARM64+amd64 Dockerfile,
  read-only root, dropped capabilities, `no-new-privileges`.
- **Deployment profiles**: `local`, `pi-lite` (Pi 4/5 tuning), `cloud`
  (Caddy auto-TLS + restic backups + postgres-exporter + prometheus),
  and a `stack.swarm.yml` for Docker Swarm with external secrets.
- **Python SDK** (`pip install ai-coop-db`): `connect`, `execute`, `select`,
  `transaction`, `vector_upsert`, `vector_search`, `rotate_key`, `me`,
  `health`. Typed error taxonomy (`AuthError`, `ValidationError`,
  `IdempotencyConflict`, `RateLimited`, `ServerError`, `NetworkError`,
  `QueueFullError`).
- **Offline retry queue** (`aicoopdb.queue.Queue`) backed by SQLite, with
  exponential backoff and a dead-letter table.
- **CLI** (`aicoopdb`): `init` (interactive onboarding wizard), `me`,
  `sql`, `key create|rotate`, `queue status|flush|clear-dead`, `doctor`.
- **Documentation**: architecture, api, security threat model, RLS guide,
  RPC authoring guide, FAQ, deploy guides, ADRs 0000–0006, and a
  16-file feature roadmap under `docs/features/`.

### Security

- TLS mandatory in any non-localhost deployment (server refuses to start
  with `AICOOPDB_INSECURE_HTTP=1` unset).
- Container runs as `USER 65532:65532` with read-only root filesystem and
  `cap_drop: [ALL]`.
- Migrations run as a separate role (`aicoopdb_owner`); the application
  server pool only ever connects as `aicoopdb_gateway`.

[Unreleased]: https://github.com/fheinfling/ai-coop-db/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fheinfling/ai-coop-db/releases/tag/v0.1.0
