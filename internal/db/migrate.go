package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // postgres driver
	_ "github.com/golang-migrate/migrate/v4/source/file"    // file:// source
	"github.com/jackc/pgx/v5"
)

// MigrationsDir returns the directory where migrations live.
//
// Resolution order:
//  1. AICOOPDB_MIGRATIONS_DIR if set
//  2. /app/migrations (the path baked into the docker image)
//  3. ./migrations relative to the working directory (dev)
func MigrationsDir() (string, error) {
	if d := os.Getenv("AICOOPDB_MIGRATIONS_DIR"); d != "" {
		return d, nil
	}
	for _, candidate := range []string{"/app/migrations", "migrations"} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", errors.New("migrations directory not found (set AICOOPDB_MIGRATIONS_DIR)")
}

// RunMigrations applies every pending migration as the role described by
// migrationsURL (typically aicoopdb_owner). It is safe to call repeatedly;
// migrate.ErrNoChange is treated as a no-op.
//
// If `password` is non-empty, it is injected into the URL via net/url so
// the operator can keep the URL string in compose / env files
// password-free and supply the secret separately (e.g. via a docker
// secret file). golang-migrate's pgx/v5 driver only takes a URL, so we
// have to embed the password in the connection string.
func RunMigrations(_ context.Context, migrationsURL, password string) error {
	if migrationsURL == "" {
		return errors.New("RunMigrations: empty migrations URL")
	}
	finalURL, err := injectPassword(migrationsURL, password)
	if err != nil {
		return fmt.Errorf("inject password: %w", err)
	}
	dir, err := MigrationsDir()
	if err != nil {
		return err
	}
	m, err := migrate.New("file://"+dir, finalURL)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate.Up: %w", err)
	}
	return nil
}

// injectPassword returns rawURL with password set, leaving the rest of the
// URL unchanged. If password is empty, the URL is returned as-is.
func injectPassword(rawURL, password string) (string, error) {
	if password == "" {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.User == nil {
		return "", fmt.Errorf("URL %q has no user component to attach a password to", rawURL)
	}
	u.User = url.UserPassword(u.User.Username(), password)
	return u.String(), nil
}

// SetRolePassword opens a short-lived connection as the migrations role
// (typically aicoopdb_owner) and runs ALTER ROLE <role> WITH PASSWORD.
//
// This is the bridge between cloud deployments and the postgres
// `scram-sha-256` default: the gateway role created by migration 0004 has
// no password until this function is called. dev profiles run postgres
// with `POSTGRES_HOST_AUTH_METHOD=trust` and skip this step entirely.
//
// `role` is validated against a tight identifier whitelist before being
// interpolated. `password` is sent as a SQL string literal — the
// PASSWORD '...' clause is parsed as a literal, not a parameter, so $1
// binding does not apply here. Single quotes are escaped by doubling per
// the SQL standard, which is exactly how Postgres parses string literals.
func SetRolePassword(ctx context.Context, migrationsURL, ownerPassword, role, newPassword string) error {
	if !isSafeIdent(role) {
		return fmt.Errorf("SetRolePassword: unsafe role identifier %q", role)
	}
	if newPassword == "" {
		return errors.New("SetRolePassword: empty new password")
	}
	finalURL, err := injectPassword(migrationsURL, ownerPassword)
	if err != nil {
		return fmt.Errorf("inject password: %w", err)
	}
	conn, err := pgx.Connect(ctx, finalURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	escaped := strings.ReplaceAll(newPassword, "'", "''")
	stmt := fmt.Sprintf(`ALTER ROLE %q WITH PASSWORD '%s'`, role, escaped)
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("alter role: %w", err)
	}
	return nil
}

// isSafeIdent returns true for identifiers consisting of lowercase letters,
// digits, and underscores. Same restriction as internal/tenant.isSafeRoleName
// — we deliberately keep the surface narrow.
func isSafeIdent(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}
