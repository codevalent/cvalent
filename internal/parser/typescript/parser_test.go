package typescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

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

func mkRun(t *testing.T, root string) *parser.Run {
	t.Helper()
	repo := &distresolver.RepoContext{
		Root:           root,
		FallbackName:   "repo:foo/bar",
		FallbackSource: model.IdentityFromRepoFallback,
	}
	return &parser.Run{
		Resolver: distresolver.New(repo, distresolver.NpmManifestSpec),
		Repo:     repo,
	}
}

func parseSource(t *testing.T, root, file, src string) []parser.FunctionNode {
	t.Helper()
	run := mkRun(t, root)
	p := New()
	nodes, err := p.Parse(run, filepath.Join(root, file), []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return nodes
}

func TestTSParser_Language(t *testing.T) {
	if New().Language() != "typescript" {
		t.Fatal("language")
	}
}

func TestTSParser_PackageJsonDistribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name": "@scope/widgets"}`)
	src := `export function frob(n: number): string { return ""; }
`
	nodes := parseSource(t, root, "src/index.ts", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1, got %d", len(nodes))
	}
	if nodes[0].Distribution != "@scope/widgets" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
}

func TestTSParser_NpmWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"workspaces": ["packages/*"]}`)
	writeFile(t, root, "packages/widgets/package.json", `{"name": "@scope/widgets"}`)
	writeFile(t, root, "packages/forms/package.json", `{"name": "@scope/forms"}`)

	src := `export function f() { return 1; }`
	a := parseSource(t, root, "packages/widgets/src/x.ts", src)
	b := parseSource(t, root, "packages/forms/index.ts", src)
	if a[0].Distribution != "@scope/widgets" {
		t.Errorf("a: %q", a[0].Distribution)
	}
	if b[0].Distribution != "@scope/forms" {
		t.Errorf("b: %q", b[0].Distribution)
	}
	if a[0].ID == b[0].ID {
		t.Errorf("different distributions must mint different UUIDs")
	}
}

func TestTSParser_ClassMethods(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name": "x"}`)
	src := `export class W {
  frob(): void {}
  baz(): void {}
}
`
	nodes := parseSource(t, root, "x.ts", src)
	if len(nodes) != 2 {
		t.Fatalf("want 2 methods, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Receiver != "W" {
			t.Errorf("receiver: %q", n.Receiver)
		}
	}
}

func TestTSParser_FallbackNoPackageJson(t *testing.T) {
	root := t.TempDir()
	src := `export function f() {}`
	nodes := parseSource(t, root, "x.ts", src)
	if len(nodes) != 1 {
		t.Fatal("want 1")
	}
	if nodes[0].IdentitySource != model.IdentityFromRepoFallback {
		t.Errorf("source: %q", nodes[0].IdentitySource)
	}
}
