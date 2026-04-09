package sql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNewExecutor_Defaults(t *testing.T) {
	cases := []struct {
		name            string
		cfg             ExecutorConfig
		wantStmtTimeout time.Duration
		wantIdleTimeout time.Duration
	}{
		{
			name:            "zero StatementTimeout defaults to 5s",
			cfg:             ExecutorConfig{IdleInTxTimeout: 10 * time.Second},
			wantStmtTimeout: 5 * time.Second,
			wantIdleTimeout: 10 * time.Second,
		},
		{
			name:            "zero IdleInTxTimeout defaults to 5s",
			cfg:             ExecutorConfig{StatementTimeout: 10 * time.Second},
			wantStmtTimeout: 10 * time.Second,
			wantIdleTimeout: 5 * time.Second,
		},
		{
			name:            "custom values preserved",
			cfg:             ExecutorConfig{StatementTimeout: 3 * time.Second, IdleInTxTimeout: 7 * time.Second},
			wantStmtTimeout: 3 * time.Second,
			wantIdleTimeout: 7 * time.Second,
		},
		{
			name:            "both zero default",
			cfg:             ExecutorConfig{},
			wantStmtTimeout: 5 * time.Second,
			wantIdleTimeout: 5 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExecutor(nil, tc.cfg)
			if e.cfg.StatementTimeout != tc.wantStmtTimeout {
				t.Errorf("StatementTimeout: got %v, want %v", e.cfg.StatementTimeout, tc.wantStmtTimeout)
			}
			if e.cfg.IdleInTxTimeout != tc.wantIdleTimeout {
				t.Errorf("IdleInTxTimeout: got %v, want %v", e.cfg.IdleInTxTimeout, tc.wantIdleTimeout)
			}
		})
	}
}

func TestClassifyPgErr_WrapsPgError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42501", Message: "permission denied"}
	got := classifyPgErr(pgErr)

	var sqlErr *Error
	if !errors.As(got, &sqlErr) {
		t.Fatalf("expected *Error, got %T", got)
	}
	if sqlErr.Pg != pgErr {
		t.Errorf("Pg field: got %p, want %p", sqlErr.Pg, pgErr)
	}
}

func TestClassifyPgErr_PlainError(t *testing.T) {
	plain := errors.New("plain")
	got := classifyPgErr(plain)
	if got != plain {
		t.Errorf("expected original error back, got %v", got)
	}
	var sqlErr *Error
	if errors.As(got, &sqlErr) {
		t.Errorf("plain error should not be wrapped in *Error")
	}
}

func TestError_NilPg(t *testing.T) {
	e := &Error{}
	want := "unknown postgres error"
	if got := e.Error(); got != want {
		t.Errorf("Error(): got %q, want %q", got, want)
	}
}

func TestError_WithPg(t *testing.T) {
	e := &Error{Pg: &pgconn.PgError{Code: "23505", Message: "unique violation"}}
	want := "23505: unique violation"
	if got := e.Error(); got != want {
		t.Errorf("Error(): got %q, want %q", got, want)
	}
}

func TestError_Unwrap(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42P01", Message: "relation does not exist"}
	e := &Error{Pg: pgErr}

	var extracted *pgconn.PgError
	if !errors.As(e, &extracted) {
		t.Fatal("errors.As failed to extract *pgconn.PgError")
	}
	if extracted != pgErr {
		t.Errorf("extracted: got %p, want %p", extracted, pgErr)
	}
}

func TestExecute_NilResult(t *testing.T) {
	e := NewExecutor(nil, ExecutorConfig{})
	_, err := e.Execute(context.Background(), ExecuteInput{Result: nil})
	if err == nil {
		t.Fatal("expected error for nil Result, got nil")
	}
	if !errors.Is(err, err) { // sanity
		t.Fatal("error identity broken")
	}
	want := "nil validator result"
	if got := err.Error(); !contains(got, want) {
		t.Errorf("error message: got %q, want substring %q", got, want)
	}
}

func TestErrResponseTooLarge_Sentinel(t *testing.T) {
	if !errors.Is(ErrResponseTooLarge, ErrResponseTooLarge) {
		t.Error("ErrResponseTooLarge should match itself")
	}
	other := errors.New("other")
	if errors.Is(other, ErrResponseTooLarge) {
		t.Error("unrelated error should not match ErrResponseTooLarge")
	}
}

// contains is a tiny helper to avoid importing strings for one check.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
