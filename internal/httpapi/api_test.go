package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fheinfling/agentic-coop-db/internal/auth"
	"github.com/fheinfling/agentic-coop-db/internal/config"
	"github.com/fheinfling/agentic-coop-db/internal/observability"
	"github.com/fheinfling/agentic-coop-db/internal/version"
)

// testAPI builds a minimal API for handler-level unit tests. Fields like
// Pool, AuthStore, Auditor, etc. are nil — tests must not reach code paths
// that dereference them.
func testAPI(t *testing.T) *API {
	t.Helper()
	cfg := &config.Config{
		RateLimitPerSecond: 100,
		RateLimitBurst:     10,
	}
	metrics := observability.NewMetrics(nil)
	return New(Deps{
		Config:  cfg,
		Logger:  slog.Default(),
		Metrics: metrics,
	})
}

// ---- handleMe ---------------------------------------------------------------

func TestHandleMe(t *testing.T) {
	a := testAPI(t)

	ws := &auth.WorkspaceContext{
		WorkspaceID: "ws-test-123",
		KeyID:       "key-abc",
		KeyDBID:     "dbid-999",
		PgRole:      "dbuser",
		Env:         auth.KeyEnvironment("production"),
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	ctx := auth.NewContext(r.Context(), ws)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	a.handleMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want it to contain application/json", ct)
	}

	var resp meResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.WorkspaceID != "ws-test-123" {
		t.Errorf("workspace_id: got %q, want %q", resp.WorkspaceID, "ws-test-123")
	}
	if resp.KeyID != "key-abc" {
		t.Errorf("key_id: got %q, want %q", resp.KeyID, "key-abc")
	}
	if resp.Role != "dbuser" {
		t.Errorf("role: got %q, want %q", resp.Role, "dbuser")
	}
	if resp.Env != "production" {
		t.Errorf("env: got %q, want %q", resp.Env, "production")
	}
	// Server info should match the version package.
	want := version.Get()
	if resp.Server != want {
		t.Errorf("server: got %+v, want %+v", resp.Server, want)
	}
}

// ---- handleKeyCreate --------------------------------------------------------

func TestHandleKeyCreate_NonAdmin(t *testing.T) {
	a := testAPI(t)

	ws := &auth.WorkspaceContext{
		WorkspaceID: "ws-aaa",
		KeyID:       "key-1",
		KeyDBID:     "dbid-1",
		PgRole:      "dbuser",
		Env:         auth.KeyEnvironment("dev"),
	}

	body := `{"workspace_id":"ws-aaa","env":"dev","pg_role":"dbuser","name":"test"}`
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/keys", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ctx := auth.NewContext(r.Context(), ws)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	a.handleKeyCreate(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "permission_denied") {
		t.Errorf("body should contain permission_denied, got: %s", w.Body.String())
	}
}

func TestHandleKeyCreate_WrongWorkspace(t *testing.T) {
	a := testAPI(t)

	ws := &auth.WorkspaceContext{
		WorkspaceID: "ws-aaa",
		KeyID:       "key-2",
		KeyDBID:     "dbid-2",
		PgRole:      "dbadmin",
		Env:         auth.KeyEnvironment("dev"),
	}

	body := `{"workspace_id":"ws-bbb","env":"dev","pg_role":"dbuser","name":"test"}`
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/keys", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ctx := auth.NewContext(r.Context(), ws)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	a.handleKeyCreate(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "permission_denied") {
		t.Errorf("body should contain permission_denied, got: %s", w.Body.String())
	}
}

func TestHandleKeyCreate_InvalidJSON(t *testing.T) {
	a := testAPI(t)

	ws := &auth.WorkspaceContext{
		WorkspaceID: "ws-aaa",
		KeyID:       "key-3",
		KeyDBID:     "dbid-3",
		PgRole:      "dbadmin",
		Env:         auth.KeyEnvironment("dev"),
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/auth/keys", strings.NewReader("not json"))
	r.Header.Set("Content-Type", "application/json")
	ctx := auth.NewContext(r.Context(), ws)
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	a.handleKeyCreate(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_json") {
		t.Errorf("body should contain invalid_json, got: %s", w.Body.String())
	}
}

// ---- clientIP ---------------------------------------------------------------

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"host:port", "1.2.3.4:8080", "1.2.3.4"},
		{"ipv6 with port", "[::1]:443", "::1"},
		{"no port", "1.2.3.4", "1.2.3.4"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			got := clientIP(r)
			if got != tc.want {
				t.Errorf("clientIP(%q): got %q, want %q", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

// ---- audit with nil Auditor -------------------------------------------------

func TestAudit_NilAuditor(t *testing.T) {
	a := testAPI(t)
	// Ensure deps.Auditor is nil (it is by default from testAPI).
	if a.deps.Auditor != nil {
		t.Fatal("test prerequisite: Auditor must be nil")
	}

	ws := &auth.WorkspaceContext{
		WorkspaceID: "ws-nil",
		KeyID:       "key-nil",
		KeyDBID:     "dbid-nil",
		PgRole:      "dbuser",
		Env:         auth.KeyEnvironment("dev"),
	}

	r := httptest.NewRequest(http.MethodPost, "/v1/sql/execute", nil)
	ctx := auth.NewContext(r.Context(), ws)
	r = r.WithContext(ctx)

	// Must not panic.
	a.audit(r, ws, "POST /v1/sql/execute", "SELECT", "SELECT 1", nil, time.Now(), http.StatusOK, nil)
}
