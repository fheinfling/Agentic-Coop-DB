BEGIN;

ALTER ROLE aicoopdb_gateway RESET search_path;
ALTER ROLE dbadmin          RESET search_path;
ALTER ROLE dbuser           RESET search_path;

ALTER TABLE IF EXISTS aicoopdb.rpc_registry     SET SCHEMA public;
ALTER TABLE IF EXISTS aicoopdb.idempotency_keys SET SCHEMA public;
ALTER TABLE IF EXISTS aicoopdb.audit_logs       SET SCHEMA public;
ALTER TABLE IF EXISTS aicoopdb.api_keys         SET SCHEMA public;
ALTER TABLE IF EXISTS aicoopdb.workspaces       SET SCHEMA public;

DROP SCHEMA IF EXISTS aicoopdb;

COMMIT;
