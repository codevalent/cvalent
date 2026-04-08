package java

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
		Resolver: distresolver.New(repo, distresolver.JavaManifestSpec),
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

func TestJavaParser_Language(t *testing.T) {
	if New().Language() != "java" {
		t.Fatal("language")
	}
}

func TestJavaParser_PomXmlDistribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pom.xml", `<project>
  <groupId>org.example</groupId>
  <artifactId>core</artifactId>
</project>`)
	src := `package org.example.widget;
public class Widget {
  public String frob(int n) { return ""; }
}
`
	nodes := parseSource(t, root, "src/main/java/org/example/widget/Widget.java", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1, got %d", len(nodes))
	}
	if nodes[0].Distribution != "org.example:core" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
	if nodes[0].Receiver != "Widget" {
		t.Errorf("receiver: %q", nodes[0].Receiver)
	}
}

func TestJavaParser_GradleDistribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "build.gradle", `group = "org.example"
rootProject.name = "core"
`)
	src := `package org.example;
public class W { public void f() {} }
`
	nodes := parseSource(t, root, "src/main/java/W.java", src)
	if len(nodes) != 1 {
		t.Fatalf("want 1")
	}
	if nodes[0].Distribution != "org.example:core" {
		t.Errorf("distribution: %q", nodes[0].Distribution)
	}
}

func TestJavaParser_OverloadDisambiguation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pom.xml", `<project>
  <groupId>x</groupId><artifactId>y</artifactId>
</project>`)
	src := `package p;
public class S {
  public String join(String[] a, String b) { return ""; }
  public String join(Iterable a, String b) { return ""; }
}
`
	nodes := parseSource(t, root, "S.java", src)
	if len(nodes) != 2 {
		t.Fatalf("want 2 overloads, got %d", len(nodes))
	}
	if nodes[0].ID == nodes[1].ID {
		t.Fatalf("overloads must mint different UUIDs")
	}
}

func TestJavaParser_FallbackNoManifest(t *testing.T) {
	root := t.TempDir()
	src := `package p;
public class W { public void f() {} }
`
	nodes := parseSource(t, root, "W.java", src)
	if len(nodes) != 1 {
		t.Fatal("want 1")
	}
	if nodes[0].IdentitySource != model.IdentityFromRepoFallback {
		t.Errorf("source: %q", nodes[0].IdentitySource)
	}
}
