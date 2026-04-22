//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fheinfling/agentic-coop-db/internal/audit"
	"github.com/fheinfling/agentic-coop-db/internal/auth"
	"github.com/fheinfling/agentic-coop-db/internal/config"
	"github.com/fheinfling/agentic-coop-db/internal/httpapi"
	mcppkg "github.com/fheinfling/agentic-coop-db/internal/mcp"
	"github.com/fheinfling/agentic-coop-db/internal/observability"
	"github.com/fheinfling/agentic-coop-db/internal/rpc"
	sqlpkg "github.com/fheinfling/agentic-coop-db/internal/sql"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestMCPEmbedded_DisabledByDefault verifies that /v1/mcp returns 404 when
// AGENTCOOPDB_MCP_ENABLED is not set (the default).
func TestMCPEmbedded_DisabledByDefault(t *testing.T) {
	h := StartHarness(t)
	// The standard harness does not mount /v1/mcp — only REST routes.
	resp, err := http.Post(h.Server.URL+"/v1/mcp", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "MCP endpoint should not exist when disabled")
}

// TestMCPEmbedded_EnabledResponds verifies that /v1/mcp accepts MCP
// requests when enabled, using the same auth as REST.
func TestMCPEmbedded_EnabledResponds(t *testing.T) {
	h := StartHarness(t)
	ctx := context.Background()

	_, token := h.MintWorkspaceAndKey(ctx, "mcp-test", "dbuser")

	// Build the MCP-enabled server from the same harness pool.
	cfg := &config.Config{
		StatementTimeout:   5 * time.Second,
		IdleInTxTimeout:    5 * time.Second,
		MaxStatementBytes:  256 * 1024,
		MaxStatementParams: 100,
		RateLimitPerSecond: 1000,
		RateLimitBurst:     2000,
	}
	logger := observability.NewLogger("error", "text")
	cache, err := auth.NewVerifyCache(32, 10*time.Millisecond)
	require.NoError(t, err)
	authMW := auth.NewMiddleware(auth.NewStore(h.Pool), cache, logger)
	metrics := observability.NewMetrics(cache.Len)
	rateLimit := httpapi.NewRateLimit(cfg.RateLimitPerSecond, cfg.RateLimitBurst)

	validator := sqlpkg.NewValidator(sqlpkg.ValidatorConfig{
		MaxStatementBytes:  cfg.MaxStatementBytes,
		MaxStatementParams: cfg.MaxStatementParams,
	})
	executor := sqlpkg.NewExecutor(h.Pool, sqlpkg.ExecutorConfig{
		StatementTimeout: cfg.StatementTimeout,
		IdleInTxTimeout:  cfg.IdleInTxTimeout,
		MaxResponseBytes: 16 * 1024 * 1024,
	})
	registry := rpc.NewRegistry()
	dispatcher := rpc.NewDispatcher(h.Pool, registry, logger)
	auditor := audit.NewWriter(h.Pool, logger, true, false)

	// Minimal doer that calls core packages directly, mirroring
	// cmd/server/direct_doer.go without auditing.
	doer := &testDirectDoer{
		pool:      h.Pool,
		validator: validator,
		executor:  executor,
		rpcDisp:   dispatcher,
		cfg:       cfg,
	}
	mcpSrv := mcppkg.NewServer(doer)
	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv,
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return r.Context()
		}),
		mcpserver.WithStateLess(true),
	)

	mux := http.NewServeMux()
	var mcpHandler http.Handler = mcpHTTP
	mcpHandler = rateLimit.Middleware(mcpHandler)
	mcpHandler = authMW.Authenticate(mcpHandler)
	mux.Handle("/v1/mcp", mcpHandler)

	// Also mount REST for comparison
	api := httpapi.New(httpapi.Deps{
		Config:         cfg,
		Logger:         logger,
		Metrics:        metrics,
		Pool:           h.Pool,
		AuthMiddleware: authMW,
		AuthStore:      auth.NewStore(h.Pool),
		Auditor:        auditor,
		Validator:      validator,
		Executor:       executor,
		RPCDispatcher:  dispatcher,
		RateLimit:      rateLimit,
	})
	mux.Handle("/v1/", http.StripPrefix("/v1", api.Routes()))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Unauthenticated request → 401
	resp, err := http.Post(srv.URL+"/v1/mcp", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "unauthenticated MCP request should be 401")

	// 2. Authenticated MCP initialize request → 200 with valid JSON-RPC response
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, err := http.NewRequest("POST", srv.URL+"/v1/mcp", strings.NewReader(initBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "authenticated MCP initialize should succeed")

	var jsonRPC map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&jsonRPC))
	assert.Equal(t, "2.0", jsonRPC["jsonrpc"], "response should be JSON-RPC 2.0")
	assert.NotNil(t, jsonRPC["result"], "initialize should return a result")

	// 3. Verify the result contains server info and capabilities
	result, ok := jsonRPC["result"].(map[string]any)
	require.True(t, ok, "result should be an object")
	serverInfo, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok, "serverInfo should be an object")
	assert.Equal(t, "agentic-coop-db", serverInfo["name"])
}

// testDirectDoer is a minimal mcp.Doer for integration tests. It mirrors
// cmd/server/direct_doer.go but lives in the test package to avoid
// importing cmd/server (which is package main).
type testDirectDoer struct {
	pool      interface{ Ping(context.Context) error }
	validator *sqlpkg.Validator
	executor  *sqlpkg.Executor
	rpcDisp   *rpc.Dispatcher
	cfg       *config.Config
}

func (d *testDirectDoer) SQLExecute(ctx context.Context, sql string, params []any, _ string) (*mcppkg.SQLResult, error) {
	ws := auth.MustFromContext(ctx)
	res, err := d.validator.Validate(sql, params)
	if err != nil {
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
		return nil, err
	}
	return &mcppkg.SQLResult{
		Command:      resp.Command,
		Columns:      resp.Columns,
		Rows:         resp.Rows,
		RowsAffected: resp.RowsAffected,
		DurationMS:   resp.DurationMS,
	}, nil
}

func (d *testDirectDoer) RPCCall(ctx context.Context, procedure string, args map[string]any) (map[string]any, error) {
	ws := auth.MustFromContext(ctx)
	res, err := d.rpcDisp.Call(ctx, rpc.CallInput{
		WorkspaceID:      ws.WorkspaceID,
		PgRole:           ws.PgRole,
		Name:             procedure,
		Args:             args,
		StatementTimeout: d.cfg.StatementTimeout,
		IdleInTxTimeout:  d.cfg.IdleInTxTimeout,
	})
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if res.Result != nil {
		_ = json.Unmarshal(res.Result, &result)
	}
	return result, nil
}

func (d *testDirectDoer) Me(ctx context.Context) (*mcppkg.MeResult, error) {
	ws := auth.MustFromContext(ctx)
	return &mcppkg.MeResult{
		WorkspaceID: ws.WorkspaceID,
		KeyID:       ws.KeyID,
		Role:        ws.PgRole,
		Env:         string(ws.Env),
	}, nil
}

func (d *testDirectDoer) Health(ctx context.Context) (*mcppkg.HealthResult, error) {
	if err := d.pool.Ping(ctx); err != nil {
		return &mcppkg.HealthResult{Healthy: false, Ready: false, Detail: err.Error()}, nil
	}
	return &mcppkg.HealthResult{Healthy: true, Ready: true}, nil
}
