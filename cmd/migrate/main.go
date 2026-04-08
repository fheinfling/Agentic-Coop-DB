// Command ai-coop-db-migrate is the standalone migration runner.
//
// Usage:
//
//	AICOOPDB_MIGRATIONS_DATABASE_URL=postgres://aicoopdb_owner@host/db?sslmode=disable \
//	AICOOPDB_OWNER_PASSWORD=...                                                         \
//	  ai-coop-db-migrate up
//
// The same logic is embedded in cmd/server when AICOOPDB_MIGRATE_ON_START=true
// (the default), so this binary is only needed when you want to run migrations
// as a one-shot job — e.g. in a kubernetes init container or a CI step.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/fheinfling/ai-coop-db/internal/db"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: ai-coop-db-migrate <up|version>

env:
  AICOOPDB_MIGRATIONS_DATABASE_URL  postgres URL (login role: aicoopdb_owner)
  AICOOPDB_OWNER_PASSWORD           password for the aicoopdb_owner role (optional; trust auth otherwise)
  AICOOPDB_OWNER_PASSWORD_FILE      file containing the same (docker secret pattern)
  AICOOPDB_MIGRATIONS_DIR           override the migrations directory
`)
	}
	flag.Parse()

	cmd := "up"
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	url := os.Getenv("AICOOPDB_MIGRATIONS_DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "AICOOPDB_MIGRATIONS_DATABASE_URL is required")
		os.Exit(2)
	}
	password := os.Getenv("AICOOPDB_OWNER_PASSWORD")
	if password == "" {
		if path := os.Getenv("AICOOPDB_OWNER_PASSWORD_FILE"); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				fail(fmt.Errorf("read AICOOPDB_OWNER_PASSWORD_FILE: %w", err))
			}
			password = string(b)
		}
	}

	switch cmd {
	case "up":
		if err := db.RunMigrations(context.Background(), url, password); err != nil {
			fail(err)
		}
		slog.Default().Info("migrations applied")
	case "version":
		fmt.Println("ai-coop-db-migrate: only `up` is currently implemented; use `migrate -database ... -path migrations version` directly for advanced flows")
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
