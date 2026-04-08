package golang

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

func mkRun(t *testing.T, root string) *parser.Run {
	t.Helper()
	repo := &distresolver.RepoContext{
		Root:           root,
		FallbackName:   "repo:foo/bar",
		FallbackSource: model.IdentityFromRepoFallback,
	}
	return &parser.Run{
		Resolver: distresolver.New(repo, distresolver.GoManifestSpec),
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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := writeAll(filepath.Join(dir, name), content); err != nil {
		t.Fatal(err)
	}
}

func TestGoParser_Language(t *testing.T) {
	if New().Language() != "go" {
		t.Fatalf("language")
	}
}

func TestGoParser_DeterministicAcrossRuns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/foo\n")
	src := `package widget
func Frob(x int) (string, error) { return "", nil }
`
	a := parseSource(t, root, "widget/x.go", src)
	b := parseSource(t, root, "widget/x.go", src)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("want 1 fn, got %d/%d", len(a), len(b))
	}
	if a[0].ID != b[0].ID {
		t.Fatalf("non-deterministic ID: %v != %v", a[0].ID, b[0].ID)
	}
}

func TestGoParser_PointerVsValueReceiver(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/foo\n")
	src := `package widget
type T struct{}
func (t T) Value() {}
func (t *T) Pointer() {}
`
	nodes := parseSource(t, root, "widget/x.go", src)
	if len(nodes) != 2 {
		t.Fatalf("want 2 methods, got %d", len(nodes))
	}
	var val, ptr parser.FunctionNode
	for _, n := range nodes {
		if n.Name == "Value" {
			val = n
		}
		if n.Name == "Pointer" {
			ptr = n
		}
	}
	if val.ID == ptr.ID {
		t.Fatalf("value and pointer receivers must mint different UUIDs")
	}
	if !ptr.PointerReceiver {
		t.Errorf("expected PointerReceiver=true on Pointer")
	}
	if val.PointerReceiver {
		t.Errorf("expected PointerReceiver=false on Value")
	}
	if !strings.Contains(ptr.QualifiedName, "(*T)") {
		t.Errorf("pointer canonical name missing (*T): %q", ptr.QualifiedName)
	}
}

func TestGoParser_GenericInstantiationsCollapse(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/foo\n")
	srcA := `package widget
type Box[T any] struct{}
func (b Box[T]) Get() T { var z T; return z }
`
	a := parseSource(t, root, "widget/a.go", srcA)
	if len(a) != 1 {
		t.Fatalf("want 1 method, got %d", len(a))
	}
	// canonical receiver should not contain "[T]"
	if strings.Contains(a[0].QualifiedName, "[") {
		t.Errorf("type params should be stripped: %q", a[0].QualifiedName)
	}
}

func TestGoParser_CrossRepoSameModule(t *testing.T) {
	// Two distinct on-disk repo roots but identical go.mod path produce
	// identical UUIDs for the same function.
	srcMod := "module example.com/shared\n"
	src := `package widget
func Frob(x int) error { return nil }
`
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeFile(t, rootA, "go.mod", srcMod)
	writeFile(t, rootB, "go.mod", srcMod)
	nodesA := parseSource(t, rootA, "widget/x.go", src)
	nodesB := parseSource(t, rootB, "widget/x.go", src)
	if nodesA[0].ID != nodesB[0].ID {
		t.Fatalf("cross-repo identity mismatch: %v != %v", nodesA[0].ID, nodesB[0].ID)
	}
}

func TestGoParser_FreeFunction(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/foo\n")
	src := `package widget
func Add(a, b int) int { return a + b }
`
	nodes := parseSource(t, root, "widget/x.go", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1, got %d", len(nodes))
	}
	if nodes[0].Name != "Add" || !nodes[0].Exported {
		t.Errorf("got %+v", nodes[0])
	}
	if nodes[0].Distribution != "example.com/foo" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
	if nodes[0].ModulePath != "widget" {
		t.Errorf("module_path: %q", nodes[0].ModulePath)
	}
}
