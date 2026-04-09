// Package vector provides a few helpers around pgvector. The package is
// intentionally tiny — most callers can write their own pgvector SQL via
// /v1/sql/execute. The helpers exist to give the SDK a stable
// db.vector_upsert / db.vector_search shape that does not require the
// caller to know the index DDL.
//
// NOTE: This package is not yet wired into an API endpoint. The Python SDK's
// vector_upsert / vector_search methods work by sending SQL through
// /v1/sql/execute directly. These server-side helpers are reserved for a
// future /v1/vector/* endpoint.
package vector
