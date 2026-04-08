package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
	goparser "github.com/codevalent/cvalent/internal/parser/golang"
)

// TestResolve_BasicGoCallEdges parses a tiny Go module with two
// functions where one calls the other, then asserts that Resolve emits
// at least one call edge linking them.
func TestResolve_BasicGoCallEdges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package widget
func helper() int { return 42 }
func main() { _ = helper() }
`
	dir := filepath.Join(root, "widget")
	_ = os.MkdirAll(dir, 0o755)
	rel := "widget/x.go"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	repo := &distresolver.RepoContext{
		Root:           root,
		FallbackName:   "repo:foo/bar",
		FallbackSource: model.IdentityFromRepoFallback,
	}
	run := &parser.Run{
		Resolver: distresolver.New(repo, distresolver.GoManifestSpec),
		Repo:     repo,
	}
	p := goparser.New()
	nodes, err := p.Parse(run, filepath.Join(root, rel), []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for i := range nodes {
		nodes[i].File = rel
	}
	edges, err := Resolve(root, nodes)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if e.CallerName == "main" && e.CalleeName == "helper" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected call edge main -> helper; got %d edges: %+v", len(edges), edges)
	}
}
