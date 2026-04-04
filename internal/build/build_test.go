package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/config"
)

func copyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "../../testdata/integration/small_project"

	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		dest := filepath.Join(dir, rel)
		if info.IsDir() {
			os.MkdirAll(dest, 0755)
			return nil
		}
		data, _ := os.ReadFile(path)
		os.MkdirAll(filepath.Dir(dest), 0755)
		os.WriteFile(dest, data, 0644)
		return nil
	})

	return dir
}

func TestBuildProducesGraph(t *testing.T) {
	dir := copyFixture(t)
	result, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	if result.FunctionCount != 4 { // 2 Go + 2 Python
		t.Fatalf("expected 4 functions, got %d", result.FunctionCount)
	}
	if result.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", result.FileCount)
	}

	// Graph file should exist
	if _, err := os.Stat(result.GraphPath); os.IsNotExist(err) {
		t.Fatal("graph file not created")
	}
}

func TestBuildAutoInitializes(t *testing.T) {
	dir := copyFixture(t)

	// No .cvalent/ directory exists
	if config.Exists(dir) {
		t.Fatal("expected no config to exist before build")
	}

	result, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Config should now exist
	if !config.Exists(dir) {
		t.Fatal("expected config to exist after build")
	}

	// Should have detected languages
	cfg, _ := config.Load(dir)
	if len(cfg.Languages) < 2 {
		t.Fatalf("expected at least 2 languages detected, got %v", cfg.Languages)
	}

	if result.FunctionCount == 0 {
		t.Fatal("expected some functions")
	}
}

func TestBuildSkippedLanguageMessage(t *testing.T) {
	dir := copyFixture(t)
	result, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	if result.Skipped[".rb"] != 1 {
		t.Fatalf("expected 1 skipped .rb file, got %d", result.Skipped[".rb"])
	}

	summary := FormatSummary(result)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	t.Log(summary)
}

func TestBuildIdempotent(t *testing.T) {
	dir := copyFixture(t)

	r1, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	r2, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	if r1.FunctionCount != r2.FunctionCount {
		t.Fatalf("idempotency: first=%d, second=%d", r1.FunctionCount, r2.FunctionCount)
	}
}

func TestBuildScopedToSubdirectory(t *testing.T) {
	dir := copyFixture(t)
	result, err := Run(Options{Root: dir, ScopePath: "go_files"})
	if err != nil {
		t.Fatal(err)
	}

	// Only Go files
	if result.FunctionCount != 2 {
		t.Fatalf("expected 2 Go functions, got %d", result.FunctionCount)
	}
}

func TestBuildContractCoverage(t *testing.T) {
	dir := copyFixture(t)
	result, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Go: 2 full, Python: 1 full + 1 inferred
	fullCount := result.ContractCoverage["full"]
	inferredCount := result.ContractCoverage["inferred"]

	if fullCount != 3 {
		t.Fatalf("expected 3 full contracts, got %d", fullCount)
	}
	if inferredCount != 1 {
		t.Fatalf("expected 1 inferred contract, got %d", inferredCount)
	}
}

func TestBuildGraphMeta(t *testing.T) {
	dir := copyFixture(t)
	result, err := Run(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	if result.FunctionCount == 0 {
		t.Fatal("expected functions")
	}
	if result.GraphPath == "" {
		t.Fatal("expected graph path")
	}
}
