package main

import (
	"context"
	"testing"

	"github.com/fheinfling/agentic-coop-db/internal/auth"
	"github.com/fheinfling/agentic-coop-db/internal/config"
	"github.com/fheinfling/agentic-coop-db/internal/mcp"
)

// authenticatedCtx returns a context carrying a test WorkspaceContext.
func authenticatedCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.WorkspaceContext{
		WorkspaceID: "ws-test-123",
		KeyID:       "k-test-456",
		KeyDBID:     "00000000-0000-0000-0000-000000000001",
		PgRole:      "dbuser",
		Env:         "test",
	})
}

func TestDirectDoer_Me(t *testing.T) {
	d := &directDoer{cfg: &config.Config{}}
	ctx := authenticatedCtx()

	me, err := d.Me(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if me.WorkspaceID != "ws-test-123" {
		t.Errorf("WorkspaceID = %q, want ws-test-123", me.WorkspaceID)
	}
	if me.KeyID != "k-test-456" {
		t.Errorf("KeyID = %q, want k-test-456", me.KeyID)
	}
	if me.Role != "dbuser" {
		t.Errorf("Role = %q, want dbuser", me.Role)
	}
	if me.Env != "test" {
		t.Errorf("Env = %q, want test", me.Env)
	}
}

func TestDirectDoer_Me_PanicsWithoutAuth(t *testing.T) {
	d := &directDoer{cfg: &config.Config{}}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unauthenticated context, got none")
		}
	}()
	_, _ = d.Me(context.Background())
}

func TestDirectDoer_Health_NilPool(t *testing.T) {
	d := &directDoer{cfg: &config.Config{}}
	result, err := d.Health(context.Background())
	if err != nil {
		t.Fatalf("Health should not return error: %v", err)
	}
	if result.Healthy {
		t.Error("expected Healthy=false when pool is nil")
	}
	if result.Ready {
		t.Error("expected Ready=false when pool is nil")
	}
	if result.Detail == "" {
		t.Error("expected non-empty Detail on unhealthy result")
	}
}

// Verify the Doer interface is satisfied at compile time.
var _ mcp.Doer = (*directDoer)(nil)

func TestDirectDoer_SQLExecute_PanicsWithNilValidator(t *testing.T) {
	// directDoer.SQLExecute calls auth.MustFromContext first, then
	// validator.Validate. With a nil validator this panics, proving
	// the auth check ran successfully before reaching the validator.
	d := &directDoer{cfg: &config.Config{}}
	ctx := authenticatedCtx()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil validator")
		}
	}()
	_, _ = d.SQLExecute(ctx, "SELECT 1", nil, "")
}

func TestDirectDoer_RPCCall_PanicsWithNilDispatcher(t *testing.T) {
	// Similar to SQL — verifies auth is checked before dispatcher is called.
	d := &directDoer{cfg: &config.Config{}}
	ctx := authenticatedCtx()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic with nil dispatcher")
		}
	}()
	_, _ = d.RPCCall(ctx, "nonexistent", nil)
}
