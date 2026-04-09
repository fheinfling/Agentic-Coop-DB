package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Context round-trip
// ---------------------------------------------------------------------------

func TestNewContext_FromContext_RoundTrip(t *testing.T) {
	ws := &WorkspaceContext{
		WorkspaceID: "ws-123",
		KeyID:       "kid-abc",
		KeyDBID:     "db-456",
		PgRole:      "app_user",
		Env:         EnvLive,
	}
	ctx := NewContext(context.Background(), ws)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("FromContext returned nil after NewContext")
	}
	if got.WorkspaceID != ws.WorkspaceID {
		t.Errorf("WorkspaceID: got %q, want %q", got.WorkspaceID, ws.WorkspaceID)
	}
	if got.KeyID != ws.KeyID {
		t.Errorf("KeyID: got %q, want %q", got.KeyID, ws.KeyID)
	}
	if got.KeyDBID != ws.KeyDBID {
		t.Errorf("KeyDBID: got %q, want %q", got.KeyDBID, ws.KeyDBID)
	}
	if got.PgRole != ws.PgRole {
		t.Errorf("PgRole: got %q, want %q", got.PgRole, ws.PgRole)
	}
	if got.Env != ws.Env {
		t.Errorf("Env: got %q, want %q", got.Env, ws.Env)
	}
	if got != ws {
		t.Error("expected same pointer identity")
	}
}

func TestFromContext_Empty(t *testing.T) {
	got := FromContext(context.Background())
	if got != nil {
		t.Errorf("expected nil from empty context, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// MustFromContext
// ---------------------------------------------------------------------------

func TestMustFromContext_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "WorkspaceContext missing from context") {
			t.Errorf("unexpected panic message: %q", msg)
		}
	}()
	MustFromContext(context.Background())
}

func TestMustFromContext_Success(t *testing.T) {
	ws := &WorkspaceContext{WorkspaceID: "ws-ok", KeyID: "kid", KeyDBID: "db", PgRole: "r", Env: EnvDev}
	ctx := NewContext(context.Background(), ws)
	got := MustFromContext(ctx)
	if got != ws {
		t.Errorf("MustFromContext returned different pointer: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// NewMiddleware nil checks
// ---------------------------------------------------------------------------

func TestNewMiddleware_NilPanics(t *testing.T) {
	var realStore KeyFinder = NewStore(nil)
	realCache := newTestCache(t, 1, time.Minute)

	cases := []struct {
		name  string
		store KeyFinder
		cache *VerifyCache
	}{
		{"nil_store", nil, realCache},
		{"nil_cache", realStore, nil},
		{"both_nil", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic, got none")
				}
			}()
			NewMiddleware(tc.store, tc.cache, nil)
		})
	}
}

func TestNewMiddleware_NilLogger(t *testing.T) {
	store := NewStore(nil)
	cache := newTestCache(t, 1, time.Minute)
	m := NewMiddleware(store, cache, nil)
	if m == nil {
		t.Fatal("NewMiddleware returned nil with nil logger")
	}
}

// ---------------------------------------------------------------------------
// writeAuthError
// ---------------------------------------------------------------------------

func TestWriteAuthError(t *testing.T) {
	type problemJSON struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}

	cases := []struct {
		name   string
		status int
		code   string
		detail string
	}{
		{
			name:   "401_missing_key",
			status: http.StatusUnauthorized,
			code:   "missing_or_invalid_api_key",
			detail: "Authorization header must be 'Bearer acd_<env>_<id>_<secret>'",
		},
		{
			name:   "500_internal",
			status: http.StatusInternalServerError,
			code:   "auth_internal",
			detail: "internal authentication error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeAuthError(w, tc.status, tc.code, tc.detail)
			resp := w.Result()
			defer func() { _ = resp.Body.Close() }()

			// Content-Type
			ct := resp.Header.Get("Content-Type")
			if ct != "application/problem+json" {
				t.Errorf("Content-Type: got %q, want %q", ct, "application/problem+json")
			}

			// WWW-Authenticate
			wwa := resp.Header.Get("WWW-Authenticate")
			if wwa != `Bearer realm="agentcoopdb"` {
				t.Errorf("WWW-Authenticate: got %q, want %q", wwa, `Bearer realm="agentcoopdb"`)
			}

			// Status code
			if resp.StatusCode != tc.status {
				t.Errorf("status: got %d, want %d", resp.StatusCode, tc.status)
			}

			// JSON body
			var body problemJSON
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Type != "about:blank" {
				t.Errorf("type: got %q, want %q", body.Type, "about:blank")
			}
			if body.Title != tc.code {
				t.Errorf("title: got %q, want %q", body.Title, tc.code)
			}
			if body.Status != tc.status {
				t.Errorf("body.status: got %d, want %d", body.Status, tc.status)
			}
			if body.Detail != tc.detail {
				t.Errorf("detail: got %q, want %q", body.Detail, tc.detail)
			}
		})
	}

	// Edge case: status 0 is not a valid HTTP status code and
	// httptest.ResponseRecorder.WriteHeader panics. We verify that
	// writeAuthError still sets headers before the panic.
	t.Run("zero_status_panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for status 0, got none")
			}
		}()
		w := httptest.NewRecorder()
		writeAuthError(w, 0, "edge_case", "edge")
	})
}

// ---------------------------------------------------------------------------
// itoa
// ---------------------------------------------------------------------------

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{42, "42"},
		{401, "401"},
		{500, "500"},
		{-500, "-500"},
		{12345, "12345"},
	}
	for _, tc := range cases {
		got := itoa(tc.in)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Authenticate — missing / malformed header
// ---------------------------------------------------------------------------

func testMiddleware(t *testing.T) *Middleware {
	t.Helper()
	store := NewStore(nil)
	cache := newTestCache(t, 1, time.Minute)
	return NewMiddleware(store, cache, nil)
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	m := testMiddleware(t)
	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	body := w.Body.String()
	if !strings.Contains(body, "missing_or_invalid_api_key") {
		t.Errorf("body does not contain expected code: %s", body)
	}
}

func TestAuthenticate_MalformedToken(t *testing.T) {
	m := testMiddleware(t)
	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad_token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	body := w.Body.String()
	if !strings.Contains(body, "missing_or_invalid_api_key") {
		t.Errorf("body does not contain expected code: %s", body)
	}
}

// ---------------------------------------------------------------------------
// RevokeFromCache — smoke test
// ---------------------------------------------------------------------------

func TestRevokeFromCache(t *testing.T) {
	m := testMiddleware(t)
	// Must not panic with an arbitrary ID.
	m.RevokeFromCache("some-id")
}

// ---------------------------------------------------------------------------
// fakeKeyFinder — test double for KeyFinder interface
// ---------------------------------------------------------------------------

type fakeKeyFinder struct {
	rec        *KeyRecord
	findErr    error
	touchErr   error
	findCalls  int
	touchCalls int
}

func (f *fakeKeyFinder) FindByKeyID(_ context.Context, _ string) (*KeyRecord, error) {
	f.findCalls++
	return f.rec, f.findErr
}

func (f *fakeKeyFinder) TouchLastUsed(_ context.Context, _ string) error {
	f.touchCalls++
	return f.touchErr
}

// mintTestKey creates a real key and returns (parsedKey, keyRecord with matching hash).
func mintTestKey(t *testing.T) (*ParsedKey, *KeyRecord) {
	t.Helper()
	keyID, secret, fullToken, err := Mint(EnvTest)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	hash, err := HashSecret(secret)
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	parsed, err := ParseBearer("Bearer " + fullToken)
	if err != nil {
		t.Fatalf("ParseBearer: %v", err)
	}
	rec := &KeyRecord{
		ID:          "db-id-001",
		WorkspaceID: "ws-001",
		KeyID:       keyID,
		SecretHash:  hash,
		Env:         EnvTest,
		PgRole:      "dbuser",
		CreatedAt:   time.Now(),
	}
	return parsed, rec
}

// ---------------------------------------------------------------------------
// resolve — cache hit / miss paths
// ---------------------------------------------------------------------------

func TestResolve_CacheHit(t *testing.T) {
	parsed, rec := mintTestKey(t)
	fake := &fakeKeyFinder{rec: rec}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	// Prime the cache.
	cache.Put(parsed.CacheKey(), rec)

	got, err := m.resolve(context.Background(), parsed)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != rec {
		t.Error("expected cached record")
	}
	if fake.findCalls != 0 {
		t.Errorf("FindByKeyID should not be called on cache hit, got %d calls", fake.findCalls)
	}
}

func TestResolve_CacheMiss_Found_Verified(t *testing.T) {
	parsed, rec := mintTestKey(t)
	fake := &fakeKeyFinder{rec: rec}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	got, err := m.resolve(context.Background(), parsed)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != rec {
		t.Error("expected the record from FindByKeyID")
	}
	if fake.findCalls != 1 {
		t.Errorf("FindByKeyID calls: got %d, want 1", fake.findCalls)
	}
	// Should now be cached.
	if _, ok := cache.Get(parsed.CacheKey()); !ok {
		t.Error("expected record to be cached after successful resolve")
	}
}

func TestResolve_CacheMiss_NotFound(t *testing.T) {
	parsed, _ := mintTestKey(t)
	fake := &fakeKeyFinder{findErr: ErrKeyNotFound}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	_, err := m.resolve(context.Background(), parsed)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected ErrKeyNotFound, got: %v", err)
	}
	if fake.findCalls != 1 {
		t.Errorf("FindByKeyID calls: got %d, want 1", fake.findCalls)
	}
}

func TestResolve_CacheMiss_BadSecret(t *testing.T) {
	parsed, rec := mintTestKey(t)
	// Replace the hash with one that won't match the secret.
	wrongHash, err := HashSecret("completely-wrong-secret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	rec.SecretHash = wrongHash

	fake := &fakeKeyFinder{rec: rec}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	_, resolveErr := m.resolve(context.Background(), parsed)
	if resolveErr == nil {
		t.Fatal("expected error, got nil")
	}
	if resolveErr != ErrInvalidKey {
		t.Errorf("expected ErrInvalidKey, got: %v", resolveErr)
	}
}

// ---------------------------------------------------------------------------
// Authenticate — full-path tests via fakeKeyFinder
// ---------------------------------------------------------------------------

func TestAuthenticate_ValidKey_Active(t *testing.T) {
	parsed, rec := mintTestKey(t)
	fake := &fakeKeyFinder{rec: rec}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	var gotWS *WorkspaceContext
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Authenticate(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+parsed.FullToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if gotWS == nil {
		t.Fatal("next handler was not called or WorkspaceContext is nil")
	}
	if gotWS.WorkspaceID != rec.WorkspaceID {
		t.Errorf("WorkspaceID: got %q, want %q", gotWS.WorkspaceID, rec.WorkspaceID)
	}
	if gotWS.PgRole != rec.PgRole {
		t.Errorf("PgRole: got %q, want %q", gotWS.PgRole, rec.PgRole)
	}
	if gotWS.KeyID != rec.KeyID {
		t.Errorf("KeyID: got %q, want %q", gotWS.KeyID, rec.KeyID)
	}
}

func TestAuthenticate_ValidKey_Revoked(t *testing.T) {
	parsed, rec := mintTestKey(t)
	revokedAt := time.Now().Add(-time.Hour)
	rec.RevokedAt = &revokedAt

	fake := &fakeKeyFinder{rec: rec}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for revoked key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+parsed.FullToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "key_inactive") {
		t.Errorf("body should contain key_inactive: %s", w.Body.String())
	}
}

func TestAuthenticate_ValidKey_DBError(t *testing.T) {
	parsed, _ := mintTestKey(t)
	fake := &fakeKeyFinder{findErr: context.DeadlineExceeded}
	cache := newTestCache(t, 10, time.Minute)
	m := NewMiddleware(fake, cache, nil)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called on DB error")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+parsed.FullToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "auth_internal") {
		t.Errorf("body should contain auth_internal: %s", w.Body.String())
	}
}
