// Package config defines the runtime configuration for the AI Coop DB gateway.
//
// All configuration is loaded from environment variables prefixed with
// AICOOPDB_. The prefix exists so AI Coop DB does not collide with other
// services running in the same compose project (e.g. POSTGRES_*).
//
// Each Config field carries an `envconfig` tag that documents its env var
// name, default, and human-readable description. `Usage()` renders the full
// reference, which is what `ai-coop-db-server -help-env` prints.
package config
