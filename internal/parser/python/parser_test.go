package python

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
		Resolver: distresolver.New(repo, distresolver.PythonManifestSpec),
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

func TestPythonParser_Language(t *testing.T) {
	if New().Language() != "python" {
		t.Fatal("language")
	}
}

func TestPythonParser_PyprojectDistribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", "[project]\nname = \"awesome_pkg\"\n")
	src := `def frob(x: int) -> str:
    return ""
`
	nodes := parseSource(t, root, "src/awesome_pkg/core.py", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1, got %d", len(nodes))
	}
	if nodes[0].Distribution != "awesome_pkg" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
	if nodes[0].IdentitySource != model.IdentityFromDistribution {
		t.Errorf("identity_source: %q", nodes[0].IdentitySource)
	}
}

func TestPythonParser_Deterministic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", "[project]\nname = \"x\"\n")
	src := `def foo(): pass
def bar(): pass
`
	a := parseSource(t, root, "x.py", src)
	b := parseSource(t, root, "x.py", src)
	if len(a) != 2 || len(b) != 2 {
		t.Fatal("want 2 nodes")
	}
	if a[0].ID != b[0].ID || a[1].ID != b[1].ID {
		t.Fatal("non-deterministic")
	}
}

func TestPythonParser_FallbackToRepo(t *testing.T) {
	root := t.TempDir()
	src := `def foo(): pass
`
	nodes := parseSource(t, root, "src/foo.py", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1, got %d", len(nodes))
	}
	if nodes[0].IdentitySource != model.IdentityFromRepoFallback {
		t.Errorf("source: %q", nodes[0].IdentitySource)
	}
	if nodes[0].Distribution != "repo:foo/bar" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
}

func TestPythonParser_ClassMethods(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", "[project]\nname = \"x\"\n")
	src := `class Widget:
    def frob(self, n: int) -> str:
        return ""
    def baz(self):
        pass
`
	nodes := parseSource(t, root, "x.py", src)
	if len(nodes) != 2 {
		t.Fatalf("want 2 methods, got %d", len(nodes))
	}
	for _, n := range nodes {
		if n.Receiver != "Widget" {
			t.Errorf("receiver: %q", n.Receiver)
		}
	}
}
