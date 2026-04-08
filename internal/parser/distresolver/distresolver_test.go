package distresolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
)

// writeFiles is a tiny helper that writes a tree of files under root.
// Map keys are relative paths; values are file contents.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeRepo builds a RepoContext rooted at dir without invoking git.
// We synthesize the git fallback name directly.
func fakeRepo(dir, fallback string, source model.IdentitySource) *RepoContext {
	return &RepoContext{Root: dir, FallbackName: fallback, FallbackSource: source}
}

func TestParseGitRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:foo/bar.git":           "foo/bar",
		"https://github.com/foo/bar.git":       "foo/bar",
		"https://github.com/foo/bar":           "foo/bar",
		"ssh://git@github.com/foo/bar.git":     "foo/bar",
		"https://gitlab.com/group/sub/project": "sub/project",
	}
	for in, want := range cases {
		got, ok := parseGitRemote(in)
		if !ok || got != want {
			t.Errorf("parseGitRemote(%q) = (%q, %v), want (%q, true)", in, got, ok, want)
		}
	}
	if _, ok := parseGitRemote(""); ok {
		t.Errorf("empty url should not parse")
	}
}

func TestResolve_GoMonorepoMultipleGoMod(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":                      "module github.com/foo/root\n",
		"services/api/go.mod":         "module github.com/foo/api\n",
		"services/api/internal/x.go":  "package internal\n",
		"services/worker/go.mod":      "module github.com/foo/worker\n",
		"services/worker/cmd/main.go": "package main\n",
		"libs/util/util.go":           "package util\n",
	})
	r := New(fakeRepo(root, "repo:foo/root", model.IdentityFromRepoFallback), GoManifestSpec)

	cases := map[string]string{
		"services/api/internal/x.go":  "github.com/foo/api",
		"services/worker/cmd/main.go": "github.com/foo/worker",
		"libs/util/util.go":           "github.com/foo/root",
	}
	for rel, want := range cases {
		d, err := r.Resolve(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if d.Name != want {
			t.Errorf("%s: got %q want %q", rel, d.Name, want)
		}
		if d.Source != model.IdentityFromDistribution {
			t.Errorf("%s: source %q", rel, d.Source)
		}
	}
}

func TestResolve_NpmWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json":                         `{"workspaces": ["packages/*"]}`,
		"packages/widgets/package.json":        `{"name": "@scope/widgets"}`,
		"packages/widgets/src/index.ts":        "export {}\n",
		"packages/widgets/src/util/helpers.ts": "export {}\n",
		"packages/forms/package.json":          `{"name": "@scope/forms"}`,
		"packages/forms/index.ts":              "export {}\n",
	})
	r := New(fakeRepo(root, "repo:foo/root", model.IdentityFromRepoFallback), NpmManifestSpec)

	d, err := r.Resolve(filepath.Join(root, "packages/widgets/src/util/helpers.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "@scope/widgets" {
		t.Errorf("got %q", d.Name)
	}
	d2, _ := r.Resolve(filepath.Join(root, "packages/forms/index.ts"))
	if d2.Name != "@scope/forms" {
		t.Errorf("got %q", d2.Name)
	}
}

func TestResolve_PythonSrcLayout(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pyproject.toml":              "[project]\nname = \"awesome_pkg\"\n",
		"src/awesome_pkg/__init__.py": "",
		"src/awesome_pkg/core.py":     "def f(): pass\n",
	})
	r := New(fakeRepo(root, "repo:foo/awesome", model.IdentityFromRepoFallback), PythonManifestSpec)
	d, err := r.Resolve(filepath.Join(root, "src/awesome_pkg/core.py"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "awesome_pkg" {
		t.Errorf("got %q", d.Name)
	}
}

func TestResolve_JavaMavenMultiModule(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"pom.xml": `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <groupId>org.example</groupId>
  <artifactId>parent</artifactId>
  <version>1.0</version>
</project>`,
		"core/pom.xml": `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <parent><groupId>org.example</groupId><artifactId>parent</artifactId><version>1.0</version></parent>
  <artifactId>core</artifactId>
</project>`,
		"core/src/main/java/Foo.java": "class Foo {}\n",
		"api/pom.xml": `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <groupId>org.example.api</groupId>
  <artifactId>api</artifactId>
</project>`,
		"api/src/main/java/Bar.java": "class Bar {}\n",
	})
	r := New(fakeRepo(root, "repo:org/example", model.IdentityFromRepoFallback), JavaManifestSpec)

	d, err := r.Resolve(filepath.Join(root, "core/src/main/java/Foo.java"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "org.example:core" {
		t.Errorf("got %q", d.Name)
	}
	d2, _ := r.Resolve(filepath.Join(root, "api/src/main/java/Bar.java"))
	if d2.Name != "org.example.api:api" {
		t.Errorf("got %q", d2.Name)
	}
}

func TestResolve_MissingManifestFallsBackToRepo(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"src/foo.ts": "export {}\n",
	})
	r := New(fakeRepo(root, "repo:foo/bar", model.IdentityFromRepoFallback), NpmManifestSpec)
	d, err := r.Resolve(filepath.Join(root, "src/foo.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "repo:foo/bar" || d.Source != model.IdentityFromRepoFallback {
		t.Errorf("got %+v", d)
	}
}

func TestResolve_NoRemoteFallback(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"src/foo.go": "package src\n",
	})
	r := New(fakeRepo(root, "repo:"+filepath.Base(root), model.IdentityFromRepoFallbackNoRemote), GoManifestSpec)
	d, _ := r.Resolve(filepath.Join(root, "src/foo.go"))
	if d.Source != model.IdentityFromRepoFallbackNoRemote {
		t.Errorf("source = %q", d.Source)
	}
}

func TestResolve_UnparseableManifestErrors(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json": "{not valid json",
		"src/foo.ts":   "",
	})
	r := New(fakeRepo(root, "repo:x/y", model.IdentityFromRepoFallback), NpmManifestSpec)
	if _, err := r.Resolve(filepath.Join(root, "src/foo.ts")); err == nil {
		t.Fatal("expected error for invalid package.json")
	}
}

func TestResolve_CacheHitsDoNotReread(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":         "module example.com/x\n",
		"a/one.go":       "package a\n",
		"a/two.go":       "package a\n",
		"a/sub/three.go": "package sub\n",
	})
	r := New(fakeRepo(root, "repo:x/y", model.IdentityFromRepoFallback), GoManifestSpec)

	// First resolve walks up. Subsequent should be hits.
	if _, err := r.Resolve(filepath.Join(root, "a/one.go")); err != nil {
		t.Fatal(err)
	}
	hits1, walks1 := r.Stats()
	if _, err := r.Resolve(filepath.Join(root, "a/two.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(filepath.Join(root, "a/sub/three.go")); err != nil {
		t.Fatal(err)
	}
	hits2, walks2 := r.Stats()
	if hits2 <= hits1 {
		t.Errorf("expected cache hits to grow: %d -> %d", hits1, hits2)
	}
	if walks2 != walks1 {
		t.Errorf("expected no further walks after cache primed: %d -> %d", walks1, walks2)
	}
}
