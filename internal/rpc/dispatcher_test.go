package rpc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// testProcedure builds a Procedure with a JSON schema requiring "id" and "body"
// (both strings). The body SQL is a dummy that is never executed in unit tests.
func testProcedure(t *testing.T, name, requiredRole string) *Procedure {
	t.Helper()
	schemaJSON := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"required": ["id", "body"],
		"properties": {
			"id":   {"type": "string"},
			"body": {"type": "string"}
		}
	}`
	schema, err := compileSchema(schemaJSON)
	if err != nil {
		t.Fatalf("compileSchema: %v", err)
	}
	return &Procedure{
		Name:         name,
		Version:      1,
		RequiredRole: requiredRole,
		Body:         "SELECT 1",
		Schema:       schema,
	}
}

func TestNewDispatcher_NilLogger(t *testing.T) {
	d := NewDispatcher(nil, NewRegistry(), nil)
	if d == nil {
		t.Fatal("NewDispatcher returned nil")
	}
	if d.logger == nil {
		t.Error("expected logger to default to slog.Default(), got nil")
	}
}

func TestCall_UnknownProcedure(t *testing.T) {
	d := NewDispatcher(nil, NewRegistry(), nil)
	_, err := d.Call(context.Background(), CallInput{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown procedure, got nil")
	}
	if !errors.Is(err, ErrUnknownProcedure) {
		t.Errorf("expected ErrUnknownProcedure, got: %v", err)
	}
}

func TestCall_RoleNotPermitted(t *testing.T) {
	reg := NewRegistry()
	proc := testProcedure(t, "admin_only", "dbadmin")
	if err := reg.Register(proc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d := NewDispatcher(nil, reg, nil)
	_, err := d.Call(context.Background(), CallInput{
		Name:   "admin_only",
		PgRole: "dbuser",
	})
	if err == nil {
		t.Fatal("expected error for role mismatch, got nil")
	}
	if !errors.Is(err, ErrRoleNotPermitted) {
		t.Errorf("expected ErrRoleNotPermitted, got: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "admin_only") {
		t.Errorf("error should contain procedure name %q, got: %s", "admin_only", msg)
	}
	if !strings.Contains(msg, "dbadmin") {
		t.Errorf("error should contain required role %q, got: %s", "dbadmin", msg)
	}
	if !strings.Contains(msg, "dbuser") {
		t.Errorf("error should contain caller role %q, got: %s", "dbuser", msg)
	}
}

func TestCall_RoleMatches(t *testing.T) {
	reg := NewRegistry()
	proc := testProcedure(t, "admin_only", "dbadmin")
	if err := reg.Register(proc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d := NewDispatcher(nil, reg, nil)

	// The call should get past role check and schema validation, then panic
	// on the nil pool when it tries to open a transaction. We recover the
	// panic to confirm the dispatcher did not reject the call early.
	var err error
	panicked := true
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil pool dereference means we got past the
				// role and schema checks.
				panicked = true
			}
		}()
		_, err = d.Call(context.Background(), CallInput{
			Name:   "admin_only",
			PgRole: "dbadmin",
			Args:   map[string]any{"id": "1", "body": "test"},
		})
		panicked = false
	}()

	if !panicked {
		// If it didn't panic, it returned an error. Make sure it is not
		// one of the early-reject sentinels.
		if err == nil {
			t.Fatal("expected an error or panic (nil pool), got nil")
		}
		if errors.Is(err, ErrRoleNotPermitted) {
			t.Error("error should NOT be ErrRoleNotPermitted when role matches")
		}
		if errors.Is(err, ErrUnknownProcedure) {
			t.Error("error should NOT be ErrUnknownProcedure for a registered procedure")
		}
	}
	// If panicked is true we are satisfied: the dispatcher reached the
	// pool-dependent code, proving the role check passed.
}

func TestCall_EmptyRequiredRole(t *testing.T) {
	reg := NewRegistry()
	proc := testProcedure(t, "open_proc", "")
	if err := reg.Register(proc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d := NewDispatcher(nil, reg, nil)

	// Same pattern as TestCall_RoleMatches: the nil pool will panic once
	// the dispatcher gets past the early checks.
	var err error
	panicked := true
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, err = d.Call(context.Background(), CallInput{
			Name:   "open_proc",
			PgRole: "anyrole",
			Args:   map[string]any{"id": "1", "body": "test"},
		})
		panicked = false
	}()

	if !panicked {
		if err == nil {
			t.Fatal("expected an error or panic (nil pool), got nil")
		}
		if errors.Is(err, ErrRoleNotPermitted) {
			t.Error("empty RequiredRole should allow any role, but got ErrRoleNotPermitted")
		}
	}
	// Panic confirms the role check was skipped as expected.
}

func TestCall_SchemaValidationFails(t *testing.T) {
	reg := NewRegistry()
	proc := testProcedure(t, "validated_proc", "")
	if err := reg.Register(proc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	d := NewDispatcher(nil, reg, nil)
	_, err := d.Call(context.Background(), CallInput{
		Name:   "validated_proc",
		PgRole: "dbuser",
		Args:   map[string]any{"body": "test"}, // missing "id"
	})
	if err == nil {
		t.Fatal("expected validation error for missing required field, got nil")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "validation") && !strings.Contains(msg, "required") {
		t.Errorf("error should mention validation or required, got: %s", err.Error())
	}
}

func TestSentinelErrors(t *testing.T) {
	if !errors.Is(ErrUnknownProcedure, ErrUnknownProcedure) {
		t.Error("ErrUnknownProcedure should match itself")
	}
	if !errors.Is(ErrRoleNotPermitted, ErrRoleNotPermitted) {
		t.Error("ErrRoleNotPermitted should match itself")
	}
	if errors.Is(ErrUnknownProcedure, ErrRoleNotPermitted) {
		t.Error("ErrUnknownProcedure must not match ErrRoleNotPermitted")
	}
	if errors.Is(ErrRoleNotPermitted, ErrUnknownProcedure) {
		t.Error("ErrRoleNotPermitted must not match ErrUnknownProcedure")
	}
}
