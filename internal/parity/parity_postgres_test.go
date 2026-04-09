//go:build parity_postgres

package parity

import (
	"os"
	"testing"
)

// TestPostgresParity_Smoke is gated behind the parity_postgres build
// tag. CI runs it via `go test -tags=parity_postgres ./...`. Local
// developers without the NEON_API_TOKEN env var skip it silently.
func TestPostgresParity_Smoke(t *testing.T) {
	token := os.Getenv("NEON_API_TOKEN")
	if token == "" {
		t.Skip("NEON_API_TOKEN not set; skipping Postgres parity")
	}
	// Future: create Neon ephemeral branch, run goose.Up against
	// Postgres migrations, compare (legacy -> SQLite -> Postgres)
	// triples. For now the placeholder ensures the build tag and CI
	// wiring compile correctly.
	t.Log("Postgres parity: branch creation + triple comparison not yet wired")
}
