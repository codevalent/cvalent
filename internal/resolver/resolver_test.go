package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/parser"
	goparser "github.com/codevalent/cvalent/internal/parser/golang"
	pyparser "github.com/codevalent/cvalent/internal/parser/python"
)

func parseGoFixture(t *testing.T, dir string, files []string) (string, []parser.FunctionNode) {
	t.Helper()
	absDir, _ := filepath.Abs(dir)
	p := goparser.New()
	var nodes []parser.FunctionNode
	for _, f := range files {
		path := filepath.Join(absDir, f)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		fns, err := p.Parse(f, src)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		nodes = append(nodes, fns...)
	}
	return absDir, nodes
}

func parsePyFixture(t *testing.T, dir string, files []string) (string, []parser.FunctionNode) {
	t.Helper()
	absDir, _ := filepath.Abs(dir)
	p := pyparser.New()
	var nodes []parser.FunctionNode
	for _, f := range files {
		path := filepath.Join(absDir, f)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		fns, err := p.Parse(f, src)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		nodes = append(nodes, fns...)
	}
	return absDir, nodes
}

func TestResolve_GoCrossFile(t *testing.T) {
	dir := "../../testdata/go/cross_file"
	files := []string{"input/types.go", "input/service.go", "input/validator.go"}
	absDir, nodes := parseGoFixture(t, dir, files)

	t.Logf("Parsed %d functions", len(nodes))
	for _, n := range nodes {
		t.Logf("  %s (%s)", n.QualifiedName, n.File)
	}

	edges, err := Resolve(absDir, nodes)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Resolved %d edges", len(edges))
	for _, e := range edges {
		t.Logf("  %s -> %s", e.CallerQualified, e.CalleeQualified)
	}

	// ProcessOrder calls validate
	found := false
	for _, e := range edges {
		if e.CallerName == "ProcessOrder" && e.CalleeName == "validate" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected edge ProcessOrder -> validate")
	}
}

func TestResolve_GoTestEdges(t *testing.T) {
	dir := "../../testdata/go/test_edges"
	files := []string{"input/service.go", "input/service_test.go"}
	absDir, nodes := parseGoFixture(t, dir, files)

	edges, err := Resolve(absDir, nodes)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Resolved %d edges", len(edges))
	for _, e := range edges {
		t.Logf("  %s -> %s", e.CallerQualified, e.CalleeQualified)
	}

	// TestProcessOrder calls ProcessOrder
	found := false
	for _, e := range edges {
		if e.CallerName == "TestProcessOrder" && e.CalleeName == "ProcessOrder" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected edge TestProcessOrder -> ProcessOrder")
	}
}

func TestResolve_PythonCrossFile(t *testing.T) {
	dir := "../../testdata/python/cross_file"
	files := []string{"input/models.py", "input/service.py", "input/validator.py"}
	absDir, nodes := parsePyFixture(t, dir, files)

	t.Logf("Parsed %d functions", len(nodes))
	for _, n := range nodes {
		t.Logf("  %s (%s)", n.QualifiedName, n.File)
	}

	edges, err := Resolve(absDir, nodes)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Resolved %d edges", len(edges))
	for _, e := range edges {
		t.Logf("  %s -> %s", e.CallerQualified, e.CalleeQualified)
	}

	// process_order calls validate
	found := false
	for _, e := range edges {
		if e.CallerName == "process_order" && e.CalleeName == "validate" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected edge process_order -> validate")
	}
}
