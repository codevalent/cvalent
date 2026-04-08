package migrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	graphdb "github.com/mstrYoda/goraphdb"
)

// seedLegacy creates a small GoraphDB store at `path` with Function
// nodes that look like Stage 0 parser output. The migrator will fall
// back to repo identity for any node whose source file no longer
// exists, which exercises the warning path.
func seedLegacy(t *testing.T, path string, fns []map[string]any) {
	t.Helper()
	gopts := graphdb.DefaultOptions()
	gopts.NoSync = true
	g, err := graphdb.Open(path, gopts)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, p := range fns {
		props := graphdb.Props{}
		for k, v := range p {
			props[k] = v
		}
		if _, err := g.AddNodeWithLabels([]string{"Function"}, props); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrate_HappyPath(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "go.mod", "module example.com/x\n")
	writeFile(t, repo, "widget/x.go", "package widget\nfunc Frob(){}\n")

	legacyPath := filepath.Join(repo, ".cvalent", "graph.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacy(t, legacyPath, []map[string]any{
		{
			"name":           "Frob",
			"qualified_name": "widget.Frob",
			"file":           "widget/x.go",
			"package":        "widget",
			"language":       "go",
			"start_line":     2,
			"end_line":       2,
			"kind":           "function",
			"exported":       true,
			"tag":            "application",
		},
	})

	newPath := filepath.Join(repo, ".cvalent", "store.db")
	res, err := Migrate(context.Background(), legacyPath, newPath, repo)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if res.MigratedCount != 1 {
		t.Errorf("MigratedCount = %d", res.MigratedCount)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("destination missing: %v", err)
	}
	if _, err := os.Stat(legacyPath + ".bak"); err != nil {
		t.Errorf("backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(legacyPath), "migration.json")); err != nil {
		t.Errorf("manifest missing: %v", err)
	}
}

func TestMigrate_RefusesWhenDestinationExists(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, repo, "go.mod", "module example.com/x\n")
	legacyPath := filepath.Join(repo, ".cvalent", "graph.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacy(t, legacyPath, []map[string]any{{"name": "F", "language": "go"}})

	newPath := filepath.Join(repo, ".cvalent", "store.db")
	if err := os.WriteFile(newPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Migrate(context.Background(), legacyPath, newPath, repo)
	if !errors.Is(err, ErrStoreExists) {
		t.Fatalf("expected ErrStoreExists, got %v", err)
	}
}

func TestMigrate_LegacyMissing(t *testing.T) {
	repo := t.TempDir()
	_, err := Migrate(context.Background(), filepath.Join(repo, "nope.db"), filepath.Join(repo, "store.db"), repo)
	if !errors.Is(err, ErrLegacyMissing) {
		t.Fatalf("got %v", err)
	}
}

func TestMigrate_RepoFallbackOnMissingSource(t *testing.T) {
	repo := t.TempDir()
	// No go.mod, no source file: legacy node will get repo_fallback.
	legacyPath := filepath.Join(repo, ".cvalent", "graph.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	seedLegacy(t, legacyPath, []map[string]any{
		{
			"name":     "Lost",
			"file":     "missing/x.go",
			"package":  "missing",
			"language": "go",
			"kind":     "function",
		},
	})
	newPath := filepath.Join(repo, ".cvalent", "store.db")
	res, err := Migrate(context.Background(), legacyPath, newPath, repo)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if res.MigratedCount != 1 {
		t.Errorf("MigratedCount = %d", res.MigratedCount)
	}
	if len(res.Warnings) == 0 {
		t.Errorf("expected at least one warning, got %d", len(res.Warnings))
	}
}
