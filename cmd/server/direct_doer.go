package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fheinfling/agentic-coop-db/internal/audit"
	"github.com/fheinfling/agentic-coop-db/internal/auth"
	"github.com/fheinfling/agentic-coop-db/internal/config"
	"github.com/fheinfling/agentic-coop-db/internal/httpapi"
	"github.com/fheinfling/agentic-coop-db/internal/mcp"
	"github.com/fheinfling/agentic-coop-db/internal/rpc"
	sqlpkg "github.com/fheinfling/agentic-coop-db/internal/sql"
)

// directDoer implements mcp.Doer by calling core packages directly,
// bypassing the HTTP round-trip that the standalone MCP binary uses.
// It provides the same semantics as the REST API — validation, tenant
// isolation via RLS, and audit logging — without network overhead.
type directDoer struct {
	pool      *pgxpool.Pool
	validator *sqlpkg.Validator
	executor  *sqlpkg.Executor
	rpcDisp   *rpc.Dispatcher
	auditor   *audit.Writer
	cfg       *config.Config
	logger    *slog.Logger
}

// idempotencyKey is accepted by the Doer interface but not used in the
// embedded path — the directDoer bypasses the HTTP layer where idempotency
// is implemented. Callers that need idempotent writes should use the REST
// API or the standalone MCP binary.
func (d *directDoer) SQLExecute(ctx context.Context, sql string, params []any, _ string) (*mcp.SQLResult, error) {
	ws := auth.MustFromContext(ctx)
	start := time.Now()

	res, err := d.validator.Validate(sql, params)
	if err != nil {
		d.audit(ctx, ws, "MCP sql_execute", "", sql, params, start, httpapi.MapError(err).Status, err)
		return nil, err
	}

	resp, err := d.executor.Execute(ctx, sqlpkg.ExecuteInput{
		WorkspaceID: ws.WorkspaceID,
		PgRole:      ws.PgRole,
		SQL:         sql,
		Params:      params,
		Result:      res,
	})
	if err != nil {
		d.audit(ctx, ws, "MCP sql_execute", res.Command, sql, params, start, httpapi.MapError(err).Status, err)
		return nil, err
	}

	d.audit(ctx, ws, "MCP sql_execute", res.Command, sql, params, start, http.StatusOK, nil)

	return &mcp.SQLResult{
		Command:      resp.Command,
		Columns:      resp.Columns,
		Rows:         resp.Rows,
		RowsAffected: resp.RowsAffected,
		DurationMS:   resp.DurationMS,
	}, nil
}

func (d *directDoer) RPCCall(ctx context.Context, procedure string, args map[string]any) (map[string]any, error) {
	ws := auth.MustFromContext(ctx)
	start := time.Now()

	res, err := d.rpcDisp.Call(ctx, rpc.CallInput{
		WorkspaceID:      ws.WorkspaceID,
		PgRole:           ws.PgRole,
		Name:             procedure,
		Args:             args,
		StatementTimeout: d.cfg.StatementTimeout,
		IdleInTxTimeout:  d.cfg.IdleInTxTimeout,
	})
	if err != nil {
		d.audit(ctx, ws, "MCP rpc_call", "RPC", procedure, []any{args}, start, httpapi.MapError(err).Status, err)
		return nil, err
	}

	var result map[string]any
	if res.Result != nil {
		if err := json.Unmarshal(res.Result, &result); err != nil {
			unmarshalErr := fmt.Errorf("unmarshal rpc result: %w", err)
			d.audit(ctx, ws, "MCP rpc_call", "RPC", procedure, []any{args}, start, http.StatusInternalServerError, unmarshalErr)
			return nil, unmarshalErr
		}
	}

	d.audit(ctx, ws, "MCP rpc_call", "RPC", procedure, []any{args}, start, http.StatusOK, nil)
	return result, nil
}

func (d *directDoer) Me(ctx context.Context) (*mcp.MeResult, error) {
	ws := auth.MustFromContext(ctx)
	return &mcp.MeResult{
		WorkspaceID: ws.WorkspaceID,
		KeyID:       ws.KeyID,
		Role:        ws.PgRole,
		Env:         string(ws.Env),
	}, nil
}

func (d *directDoer) Health(ctx context.Context) (*mcp.HealthResult, error) {
	if d.pool == nil {
		return &mcp.HealthResult{
			Healthy: false,
			Ready:   false,
			Detail:  "database pool not initialized",
		}, nil
	}
	err := d.pool.Ping(ctx)
	if err != nil {
		return &mcp.HealthResult{
			Healthy: false,
			Ready:   false,
			Detail:  fmt.Sprintf("database ping failed: %v", err),
		}, nil
	}
	return &mcp.HealthResult{Healthy: true, Ready: true}, nil
}

func (d *directDoer) audit(ctx context.Context, ws *auth.WorkspaceContext, endpoint, command, sql string, params []any, start time.Time, status int, err error) {
	if d.auditor == nil {
		return
	}
	var errCode string
	if err != nil {
		problem := httpapi.MapError(err)
		errCode = problem.Title
	}
	d.auditor.Write(ctx, audit.Entry{
		RequestID:   httpapi.GetRequestID(ctx),
		WorkspaceID: ws.WorkspaceID,
		KeyDBID:     ws.KeyDBID,
		Endpoint:    endpoint,
		Command:     command,
		SQL:         sql,
		Params:      params,
		DurationMS:  int(time.Since(start).Milliseconds()),
		StatusCode:  status,
		ErrorCode:   errCode,
	})
}
