//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnparseableSQLRejected(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()
	_, token := h.MintWorkspaceAndKey(ctx, "ws-parse", "dbadmin")

	resp := postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "NOT VALID SQL!!!",
		"params": []any{},
	})
	require.Equal(t, http.StatusBadRequest, resp["__status"])
	require.Equal(t, "parse_error", resp["title"])
}

func TestParamsMismatchRejected(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()
	_, token := h.MintWorkspaceAndKey(ctx, "ws-params", "dbadmin")

	resp := postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "SELECT $1, $2",
		"params": []any{"only-one"},
	})
	require.Equal(t, http.StatusBadRequest, resp["__status"])
	require.Equal(t, "params_mismatch", resp["title"])
}

func TestMultipleStatementsRejected(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()
	_, token := h.MintWorkspaceAndKey(ctx, "ws-multi", "dbadmin")

	resp := postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "SELECT 1; DROP TABLE workspaces;",
		"params": []any{},
	})
	require.Equal(t, http.StatusBadRequest, resp["__status"])
	require.Equal(t, "multiple_statements", resp["title"])
}

func TestSelectRoundTrip(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()
	_, token := h.MintWorkspaceAndKey(ctx, "ws-select", "dbadmin")

	resp := postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "SELECT 1 AS one, $1::text AS hello",
		"params": []any{"world"},
	})
	require.Equal(t, http.StatusOK, resp["__status"])
	require.Equal(t, "SELECT", resp["command"])
}

func TestInsertReturningReturnsRows(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()
	_, token := h.MintWorkspaceAndKey(ctx, "ws-returning", "dbadmin")

	// Create a test table.
	resp := postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "CREATE TABLE test_returning (id serial PRIMARY KEY, name text NOT NULL)",
		"params": []any{},
	})
	require.Equal(t, http.StatusOK, resp["__status"])

	// INSERT ... RETURNING should populate columns and rows.
	resp = postJSON(t, h, token, "/v1/sql/execute", map[string]any{
		"sql":    "INSERT INTO test_returning (name) VALUES ($1) RETURNING id, name",
		"params": []any{"hello"},
	})
	require.Equal(t, http.StatusOK, resp["__status"])
	require.Equal(t, "INSERT", resp["command"])

	cols, ok := resp["columns"].([]any)
	require.True(t, ok, "columns should be an array, got %T", resp["columns"])
	require.Equal(t, []any{"id", "name"}, cols)

	rows, ok := resp["rows"].([]any)
	require.True(t, ok, "rows should be an array, got %T", resp["rows"])
	require.Len(t, rows, 1)

	row := rows[0].([]any)
	require.Equal(t, "hello", row[1])
	require.Equal(t, float64(1), resp["rows_affected"])
}
