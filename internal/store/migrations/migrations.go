// Package migrations embeds the SQL migration files for goose to run on
// Store.Open(). The actual store wrapper that calls goose lives in
// internal/store; this package just exposes the embedded filesystems.
package migrations

import "embed"

//go:embed sqlite/*.sql
var sqliteFS embed.FS

//go:embed postgres/*.sql
var postgresFS embed.FS

// SQLite returns the embedded SQLite migration filesystem rooted at the
// "sqlite" subdirectory.
func SQLite() embed.FS { return sqliteFS }

// Postgres returns the embedded Postgres migration filesystem rooted at
// the "postgres" subdirectory.
func Postgres() embed.FS { return postgresFS }
