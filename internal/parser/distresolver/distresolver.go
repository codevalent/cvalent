// Package distresolver walks upward from any source file to the nearest
// distribution manifest (go.mod, package.json, pyproject.toml/setup.cfg/
// setup.py, pom.xml/build.gradle*) and returns the distribution name.
//
// It is shared by all four parsers and by the migrator. There is exactly
// one walker, one cache, and one git-remote-fallback rule. Parsers
// inject a ManifestSpec for their language and call Resolve(filePath).
//
// Failure modes:
//
//   - Manifest found but unparseable → error (parser decides whether to
//     fall through to repo fallback or fail loudly).
//   - No manifest, git remote present → repo:<account>/<repo>, source =
//     repo_fallback.
//   - No manifest, no git remote      → repo:<basename>, source =
//     repo_fallback_no_remote.
package distresolver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/codevalent/cvalent/internal/model"
)

// Distribution is the resolver's output for a single source file.
type Distribution struct {
	// Name is the distribution string suitable for use as
	// IdentityParts.Distribution. Examples: "github.com/foo/bar",
	// "org.apache.commons:commons-lang3", "python-dotenv",
	// "@scope/package-name", "repo:foo/bar", "repo:bar".
	Name string

	// Source records how the distribution was resolved.
	Source model.IdentitySource

	// ManifestPath is the absolute path of the manifest file used, or the
	// empty string for repo fallbacks.
	ManifestPath string
}

// ManifestSpec describes a single language's manifest format.
type ManifestSpec struct {
	// Filenames is the ordered list of filenames considered for this
	// language. The first filename found in a directory wins.
	Filenames []string

	// Parse extracts the distribution name from the bytes of a manifest
	// file. Return an empty string with no error if the file does not
	// declare a distribution name (e.g. a stub package.json with no
	// "name" field) — the resolver will continue walking upward.
	Parse func(path string, data []byte) (string, error)
}

// RepoContext is the per-run repo state used to compute fallbacks. It is
// resolved once at parser run start.
type RepoContext struct {
	// Root is the absolute path of the repo root (the directory that
	// contains .git, or the highest directory walked if no .git is
	// found). Used as the upward walk terminator.
	Root string

	// FallbackName is the value used for the repo fallback. It is one
	// of "repo:<account>/<repo>" (when git remote was found) or
	// "repo:<basename>" (when no remote).
	FallbackName string

	// FallbackSource matches FallbackName.
	FallbackSource model.IdentitySource
}

// NewRepoContext discovers the git root for repoPath, parses
// `git remote get-url origin`, and computes the fallback name.
//
// repoPath may be a file or a directory; the resolver walks upward from
// it to find .git. If no .git is found, the highest directory walked
// becomes Root and FallbackName is "repo:<basename>".
func NewRepoContext(repoPath string) (*RepoContext, error) {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	startDir := abs
	if !info.IsDir() {
		startDir = filepath.Dir(abs)
	}

	root := findGitRoot(startDir)
	if root == "" {
		// No .git anywhere — use startDir as root, fallback to basename.
		return &RepoContext{
			Root:           startDir,
			FallbackName:   "repo:" + filepath.Base(startDir),
			FallbackSource: model.IdentityFromRepoFallbackNoRemote,
		}, nil
	}

	remote, ok := readGitRemote(root)
	if !ok {
		return &RepoContext{
			Root:           root,
			FallbackName:   "repo:" + filepath.Base(root),
			FallbackSource: model.IdentityFromRepoFallbackNoRemote,
		}, nil
	}

	return &RepoContext{
		Root:           root,
		FallbackName:   "repo:" + remote,
		FallbackSource: model.IdentityFromRepoFallback,
	}, nil
}

func findGitRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// readGitRemote runs `git remote get-url origin` in repoRoot and returns
// the cleaned account/repo string. ok=false if there is no remote or git
// is unavailable.
func readGitRemote(repoRoot string) (string, bool) {
	cmd := exec.Command("git", "-C", repoRoot, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return parseGitRemote(strings.TrimSpace(string(out)))
}

// parseGitRemote strips protocol and `.git` and returns the last two
// path segments joined by `/`. Exposed for testing.
func parseGitRemote(url string) (string, bool) {
	if url == "" {
		return "", false
	}
	// SSH form: git@github.com:account/repo.git
	if strings.HasPrefix(url, "git@") {
		idx := strings.Index(url, ":")
		if idx == -1 {
			return "", false
		}
		url = url[idx+1:]
	} else {
		// HTTPS form: https://github.com/account/repo[.git]
		for _, prefix := range []string{"https://", "http://", "git://", "ssh://"} {
			if strings.HasPrefix(url, prefix) {
				url = strings.TrimPrefix(url, prefix)
				break
			}
		}
		// Drop the host.
		if idx := strings.Index(url, "/"); idx != -1 {
			url = url[idx+1:]
		}
	}
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return "", false
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1], true
}

// Resolver walks upward from a source file to the nearest manifest and
// caches per-directory results.
type Resolver struct {
	repo  *RepoContext
	spec  ManifestSpec
	cache sync.Map // map[string]Distribution

	hits  int64
	walks int64
	mu    sync.Mutex
}

// New constructs a Resolver for one language.
func New(repo *RepoContext, spec ManifestSpec) *Resolver {
	return &Resolver{repo: repo, spec: spec}
}

// Stats returns hit and walk counters for testing.
func (r *Resolver) Stats() (hits, walks int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits, r.walks
}

// Resolve walks upward from filePath looking for a manifest. Returns
// the Distribution. The walk terminates at the repo root.
//
// On cache hit (a sibling file under the same manifest directory), no
// filesystem reads happen.
func (r *Resolver) Resolve(filePath string) (Distribution, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return Distribution{}, err
	}
	dir := filepath.Dir(abs)

	// Walk upward, checking the cache for each directory before reading.
	visited := []string{}
	for {
		if cached, ok := r.cache.Load(dir); ok {
			r.mu.Lock()
			r.hits++
			r.mu.Unlock()
			d := cached.(Distribution)
			r.fillCacheChain(visited, d)
			return d, nil
		}
		visited = append(visited, dir)

		// Try to read a manifest from this directory.
		if d, found, err := r.tryDir(dir); err != nil {
			return Distribution{}, fmt.Errorf("distresolver: %w", err)
		} else if found {
			r.mu.Lock()
			r.walks++
			r.mu.Unlock()
			r.fillCacheChain(visited, d)
			return d, nil
		}

		// Stop at the repo root.
		if dir == r.repo.Root {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// No manifest found anywhere — repo fallback.
	d := Distribution{
		Name:   r.repo.FallbackName,
		Source: r.repo.FallbackSource,
	}
	r.fillCacheChain(visited, d)
	r.mu.Lock()
	r.walks++
	r.mu.Unlock()
	return d, nil
}

// fillCacheChain installs the same Distribution under every directory in
// `chain` so that subsequent sibling lookups become cache hits.
func (r *Resolver) fillCacheChain(chain []string, d Distribution) {
	for _, dir := range chain {
		r.cache.Store(dir, d)
	}
}

// tryDir attempts to read each manifest filename in `dir` and parse it.
// Returns (distribution, true, nil) on success, (zero, false, nil) if
// no manifest in this directory, or an error if a manifest exists but is
// unparseable.
func (r *Resolver) tryDir(dir string) (Distribution, bool, error) {
	for _, fname := range r.spec.Filenames {
		path := filepath.Join(dir, fname)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Distribution{}, false, err
		}
		name, err := r.spec.Parse(path, data)
		if err != nil {
			return Distribution{}, false, fmt.Errorf("parse %s: %w", path, err)
		}
		if name == "" {
			// Manifest exists but does not declare a name — keep walking
			// upward (e.g. a workspace root with no own package).
			continue
		}
		return Distribution{
			Name:         name,
			Source:       model.IdentityFromDistribution,
			ManifestPath: path,
		}, true, nil
	}
	return Distribution{}, false, nil
}
