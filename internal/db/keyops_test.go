package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fheinfling/agentic-coop-db/internal/auth"
)

func TestKeyStatus(t *testing.T) {
	now := time.Now()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	cases := []struct {
		name      string
		revokedAt *time.Time
		expiresAt *time.Time
		want      string
	}{
		{"nil/nil is active", nil, nil, "active"},
		{"revoked", &past, nil, "revoked"},
		{"revoked takes precedence over expired", &past, &past, "revoked"},
		{"expired in the past", nil, &past, "expired"},
		{"expired exactly now", nil, &now, "expired"},
		{"future expiry is active", nil, &future, "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := keyStatus(tc.revokedAt, tc.expiresAt)
			if got != tc.want {
				t.Errorf("keyStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRevokeKeyValidatesUUID(t *testing.T) {
	bad := []string{"", "not-a-uuid", "'; DROP TABLE api_keys;--"}
	for _, id := range bad {
		t.Run(id, func(t *testing.T) {
			err := RevokeKey(context.Background(), "postgres://localhost/test", "", id)
			if err == nil {
				t.Fatal("expected error for invalid UUID")
			}
		})
	}
}

func TestMintKey_EmptyWorkspace(t *testing.T) {
	_, err := MintKey(context.Background(), "postgres://localhost/test", "pass", "", "dbuser", auth.EnvDev)
	if err == nil {
		t.Fatal("expected error for empty workspace")
	}
	if !strings.Contains(err.Error(), "empty workspace") {
		t.Errorf("error = %q, want it to contain %q", err, "empty workspace")
	}
}

func TestMintKey_EmptyPgRole(t *testing.T) {
	_, err := MintKey(context.Background(), "postgres://localhost/test", "pass", "ws", "", auth.EnvDev)
	if err == nil {
		t.Fatal("expected error for empty pg_role")
	}
	if !strings.Contains(err.Error(), "empty pg_role") {
		t.Errorf("error = %q, want it to contain %q", err, "empty pg_role")
	}
}

func TestMintKey_InvalidEnv(t *testing.T) {
	_, err := MintKey(context.Background(), "postgres://localhost/test", "pass", "ws", "dbuser", auth.KeyEnvironment("staging"))
	if err == nil {
		t.Fatal("expected error for invalid env")
	}
	if !strings.Contains(err.Error(), "invalid env") {
		t.Errorf("error = %q, want it to contain %q", err, "invalid env")
	}
}
