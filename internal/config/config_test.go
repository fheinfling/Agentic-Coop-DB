package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper that wipes every AGENTCOOPDB_* var so a Load() call sees a known
// empty environment, then restores the original values when done.
func clearAGENTCOOPDBEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			k := kv[:eq]
			if strings.HasPrefix(k, "AGENTCOOPDB_") {
				t.Setenv(k, "")
			}
		}
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("Load() with no AGENTCOOPDB_DATABASE_URL: expected error, got nil")
	}
}

func TestLoad_DefaultsAndOverrides(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@localhost/agentcoopdb?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Sample of defaults that should be in effect.
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default: got %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.StatementTimeout != 5*time.Second {
		t.Errorf("StatementTimeout default: got %s, want 5s", cfg.StatementTimeout)
	}
	if cfg.MetricsEnabled != true {
		t.Errorf("MetricsEnabled default: got %v, want true", cfg.MetricsEnabled)
	}
	if cfg.MigrateOnStart != true {
		t.Errorf("MigrateOnStart default: got %v, want true", cfg.MigrateOnStart)
	}
	// MIGRATIONS_DATABASE_URL should default to DATABASE_URL when unset.
	if cfg.MigrationsDatabaseURL != cfg.DatabaseURL {
		t.Errorf("MigrationsDatabaseURL default: got %q, want it to mirror DatabaseURL %q",
			cfg.MigrationsDatabaseURL, cfg.DatabaseURL)
	}
}

func TestLoad_MigrationsURLDistinctFromDatabaseURL(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_MIGRATIONS_DATABASE_URL", "postgres://agentcoopdb_owner@host/db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL == cfg.MigrationsDatabaseURL {
		t.Errorf("DatabaseURL and MigrationsDatabaseURL must remain distinct when both are set")
	}
	if !strings.Contains(cfg.MigrationsDatabaseURL, "agentcoopdb_owner") {
		t.Errorf("MigrationsDatabaseURL: got %q", cfg.MigrationsDatabaseURL)
	}
}

func TestLoad_RejectsStatementTimeoutAbove60s(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_STATEMENT_TIMEOUT", "61s")
	if _, err := Load(); err == nil {
		t.Fatal("Load with statement_timeout=61s: expected error, got nil")
	}
}

func TestLoad_RejectsZeroMaxStatementParams(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_MAX_STATEMENT_PARAMS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load with MAX_STATEMENT_PARAMS=0: expected error, got nil")
	}
}

func TestLoad_OwnerPasswordFromFile(t *testing.T) {
	clearAGENTCOOPDBEnv(t)

	dir := t.TempDir()
	pwFile := filepath.Join(dir, "owner_password.txt")
	if err := os.WriteFile(pwFile, []byte("file-password\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_OWNER_PASSWORD_FILE", pwFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OwnerPassword != "file-password" {
		t.Errorf("OwnerPassword from file: got %q, want %q", cfg.OwnerPassword, "file-password")
	}
}

func TestLoad_DirectPasswordWinsOverFile(t *testing.T) {
	clearAGENTCOOPDBEnv(t)

	dir := t.TempDir()
	pwFile := filepath.Join(dir, "owner.txt")
	if err := os.WriteFile(pwFile, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_OWNER_PASSWORD", "from-env")
	t.Setenv("AGENTCOOPDB_OWNER_PASSWORD_FILE", pwFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OwnerPassword != "from-env" {
		t.Errorf("OwnerPassword: direct env var must win over the *_FILE variant; got %q", cfg.OwnerPassword)
	}
}

func TestLoad_GatewayPasswordFromFile(t *testing.T) {
	// Symmetric with the owner test — both secrets share the same
	// loadSecretFromFile path, so this is mostly a sanity check that
	// the wiring is in place for both.
	clearAGENTCOOPDBEnv(t)

	dir := t.TempDir()
	pwFile := filepath.Join(dir, "gw.txt")
	if err := os.WriteFile(pwFile, []byte("  gateway-pw  \n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_GATEWAY_PASSWORD_FILE", pwFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GatewayPassword != "gateway-pw" {
		t.Errorf("GatewayPassword: surrounding whitespace should be trimmed; got %q", cfg.GatewayPassword)
	}
}

func TestLoad_PasswordFileMissingFails(t *testing.T) {
	clearAGENTCOOPDBEnv(t)
	t.Setenv("AGENTCOOPDB_DATABASE_URL", "postgres://agentcoopdb_gateway@host/db")
	t.Setenv("AGENTCOOPDB_OWNER_PASSWORD_FILE", "/this/path/definitely/does/not/exist")
	if _, err := Load(); err == nil {
		t.Fatal("Load with missing OWNER_PASSWORD_FILE: expected error, got nil")
	}
}

func TestUsage_RendersNonEmpty(t *testing.T) {
	// The output should be a non-empty string and should mention at
	// least one of our env var names so we know it actually rendered
	// the spec, not an error placeholder.
	got := Usage()
	if got == "" {
		t.Fatal("Usage() returned empty string")
	}
	if !strings.Contains(got, "AGENTCOOPDB_") {
		t.Errorf("Usage() output should contain AGENTCOOPDB_ env var names; got:\n%s", got)
	}
	if strings.Contains(got, "failed to render usage") {
		t.Errorf("Usage() returned the error placeholder: %q", got)
	}
}

// ---- envOr ------------------------------------------------------------------

func TestEnvOr(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		envVal   string
		set      bool
		fallback string
		want     string
	}{
		{"env var set", "TEST_ENVOR_SET", "from_env", true, "fallback", "from_env"},
		{"env var not set", "TEST_ENVOR_UNSET", "", false, "fallback", "fallback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(tc.key, tc.envVal)
			}
			got := envOr(tc.key, tc.fallback)
			if got != tc.want {
				t.Errorf("envOr(%q, %q): got %q, want %q", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}

// ---- envBool ----------------------------------------------------------------

func TestEnvBool(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		envVal   string
		set      bool
		fallback bool
		want     bool
	}{
		{"true", "TEST_ENVBOOL_1", "true", true, false, true},
		{"false", "TEST_ENVBOOL_2", "false", true, true, false},
		{"1", "TEST_ENVBOOL_3", "1", true, false, true},
		{"0", "TEST_ENVBOOL_4", "0", true, true, false},
		{"invalid returns fallback", "TEST_ENVBOOL_5", "invalid", true, true, true},
		{"unset returns fallback", "TEST_ENVBOOL_6", "", false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(tc.key, tc.envVal)
			}
			got := envBool(tc.key, tc.fallback)
			if got != tc.want {
				t.Errorf("envBool(%q, %v): got %v, want %v", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}

// ---- envDuration ------------------------------------------------------------

func TestEnvDuration(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		envVal   string
		set      bool
		fallback time.Duration
		want     time.Duration
		wantErr  bool
	}{
		{"5s", "TEST_ENVDUR_1", "5s", true, time.Second, 5 * time.Second, false},
		{"100ms", "TEST_ENVDUR_2", "100ms", true, time.Second, 100 * time.Millisecond, false},
		{"invalid returns error", "TEST_ENVDUR_3", "invalid", true, 3 * time.Second, 0, true},
		{"unset returns fallback", "TEST_ENVDUR_4", "", false, 7 * time.Second, 7 * time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(tc.key, tc.envVal)
			}
			got, err := envDuration(tc.key, tc.fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("envDuration(%q, %v): got %v, want %v", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}

// ---- envInt -----------------------------------------------------------------

func TestEnvInt(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		envVal   string
		set      bool
		fallback int
		want     int
		wantErr  bool
	}{
		{"42", "TEST_ENVINT_1", "42", true, 0, 42, false},
		{"not a number returns error", "TEST_ENVINT_2", "notanumber", true, 99, 0, true},
		{"unset returns fallback", "TEST_ENVINT_3", "", false, 99, 99, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(tc.key, tc.envVal)
			}
			got, err := envInt(tc.key, tc.fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("envInt(%q, %d): got %d, want %d", tc.key, tc.fallback, got, tc.want)
			}
		})
	}
}
