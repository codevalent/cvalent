// Package migrator implements the legacy GoraphDB → SQLite store
// migration that powers `cvalent migrate-store`.
//
// The CLI subcommand wiring lives in cmd/cvalent and lands as
// AH-0316.18; this package is the pure-Go core. It is the only
// remaining importer of `goraphdb` after Stage C — kept here so the
// migrator can read pre-Rung-0 graph.db files. Once every install has
// migrated, this package and goraphdb can be removed together.
package migrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	graphdb "github.com/mstrYoda/goraphdb"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
	"github.com/codevalent/cvalent/internal/store"
)

// ErrStoreExists is returned when Migrate is called and the destination
// store.db already exists. The migrator refuses to overwrite an
// existing destination — users must delete it explicitly.
var ErrStoreExists = errors.New("migrator: destination store already exists")

// ErrLegacyMissing is returned when the source GoraphDB file does not
// exist.
var ErrLegacyMissing = errors.New("migrator: legacy graph not found")

// Migrate reads the legacy GoraphDB store at legacyPath, normalizes
// every node's identity through model.Canonicalize, and writes the
// result into a new SQLite store at newPath.
//
// Behavior:
//   - Refuses to run if newPath already exists (returns ErrStoreExists).
//   - Wraps the entire write in a single SQLite transaction; partial
//     failure leaves no destination on disk.
//   - On success, renames legacyPath to legacyPath+".bak" and writes
//     filepath.Dir(legacyPath)+"/migration.json" with timestamps and
//     paths.
//   - Re-resolves each legacy node's distribution from `repoPath` via
//     distresolver. Nodes that cannot be re-resolved are written with
//     IdentitySource = repo_fallback and a warning is appended to
//     `Warnings` on the returned Result.
func Migrate(ctx context.Context, legacyPath, newPath, repoPath string) (*Result, error) {
	res := &Result{LegacyPath: legacyPath, NewPath: newPath}

	if _, err := os.Stat(legacyPath); errors.Is(err, os.ErrNotExist) {
		return nil, ErrLegacyMissing
	} else if err != nil {
		return nil, err
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil, ErrStoreExists
	}

	repoCtx, err := distresolver.NewRepoContext(repoPath)
	if err != nil {
		return nil, fmt.Errorf("migrator: repo context: %w", err)
	}

	gopts := graphdb.DefaultOptions()
	gopts.NoSync = true
	g, err := graphdb.Open(legacyPath, gopts)
	if err != nil {
		return nil, fmt.Errorf("migrator: open legacy: %w", err)
	}
	defer g.Close()

	legacyNodes, err := readLegacyFunctions(g)
	if err != nil {
		return nil, fmt.Errorf("migrator: read legacy: %w", err)
	}
	res.LegacyCount = len(legacyNodes)

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return nil, err
	}
	dstStore, err := store.Open(ctx, newPath)
	if err != nil {
		return nil, fmt.Errorf("migrator: open destination: %w", err)
	}
	defer dstStore.Close()

	tx, err := dstStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
			_ = os.Remove(newPath)
		}
	}()

	resolvers := newResolverCache(repoCtx)
	for _, ln := range legacyNodes {
		fn, warning, err := normalizeOne(ln, resolvers)
		if err != nil {
			return nil, fmt.Errorf("migrator: normalize %s: %w", ln.QualifiedName, err)
		}
		if warning != "" {
			res.Warnings = append(res.Warnings, warning)
		}
		if err := dstStore.UpsertNodeTx(ctx, tx, fn); err != nil {
			return nil, fmt.Errorf("migrator: write %s: %w", fn.QualifiedName, err)
		}
		res.MigratedCount++
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true

	// Backup the legacy file and write migration.json.
	if err := os.Rename(legacyPath, legacyPath+".bak"); err != nil {
		return res, fmt.Errorf("migrator: backup rename: %w", err)
	}
	res.BackupPath = legacyPath + ".bak"

	manifest := map[string]any{
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"legacy_path": legacyPath,
		"new_path":    newPath,
		"backup_path": res.BackupPath,
		"migrated":    res.MigratedCount,
		"warnings":    len(res.Warnings),
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(filepath.Dir(legacyPath), "migration.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return res, fmt.Errorf("migrator: write manifest: %w", err)
	}
	res.ManifestPath = manifestPath

	return res, nil
}

// Result captures the migration outcome.
type Result struct {
	LegacyPath    string
	NewPath       string
	BackupPath    string
	ManifestPath  string
	LegacyCount   int
	MigratedCount int
	Warnings      []string
}

// legacyFunction is the trimmed-down view of a Function row read from
// goraphdb. The legacy schema has many properties; we only need the
// ones that participate in identity normalization or carry-over data.
type legacyFunction struct {
	Name                 string
	QualifiedName        string
	File                 string
	Package              string
	Module               string
	Language             string
	StartLine            int
	EndLine              int
	Kind                 string
	Receiver             string
	Exported             bool
	Tag                  string
	ContractCompleteness string
}

// readLegacyFunctions iterates the legacy store and returns one
// legacyFunction per Function-labelled node.
func readLegacyFunctions(g *graphdb.DB) ([]legacyFunction, error) {
	result, err := g.Cypher(context.Background(), `MATCH (f) RETURN f`)
	if err != nil {
		return nil, err
	}
	out := make([]legacyFunction, 0, len(result.Rows))
	for _, row := range result.Rows {
		raw, ok := row["f"].(*graphdb.Node)
		if !ok || raw == nil {
			continue
		}
		props := raw.Props
		if !hasFunctionLabel(raw) {
			continue
		}
		out = append(out, legacyFunction{
			Name:                 stringProp(props, "name"),
			QualifiedName:        stringProp(props, "qualified_name"),
			File:                 stringProp(props, "file"),
			Package:              stringProp(props, "package"),
			Module:               stringProp(props, "module"),
			Language:             stringProp(props, "language"),
			StartLine:            intProp(props, "start_line"),
			EndLine:              intProp(props, "end_line"),
			Kind:                 stringProp(props, "kind"),
			Receiver:             stringProp(props, "receiver"),
			Exported:             boolProp(props, "exported"),
			Tag:                  stringProp(props, "tag"),
			ContractCompleteness: stringProp(props, "contract_completeness"),
		})
	}
	return out, nil
}

func hasFunctionLabel(n *graphdb.Node) bool {
	for _, l := range n.Labels {
		if l == "Function" {
			return true
		}
	}
	return false
}

func stringProp(props graphdb.Props, key string) string {
	v, ok := props[key]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intProp(props graphdb.Props, key string) int {
	v, ok := props[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func boolProp(props graphdb.Props, key string) bool {
	v, ok := props[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// resolverCache holds one distresolver per language so that the
// migrator can re-resolve any legacy node by language without
// reconstructing a Resolver per call.
type resolverCache struct {
	repo *distresolver.RepoContext
	r    map[string]*distresolver.Resolver
}

func newResolverCache(repo *distresolver.RepoContext) *resolverCache {
	return &resolverCache{
		repo: repo,
		r: map[string]*distresolver.Resolver{
			"go":         distresolver.New(repo, distresolver.GoManifestSpec),
			"java":       distresolver.New(repo, distresolver.JavaManifestSpec),
			"python":     distresolver.New(repo, distresolver.PythonManifestSpec),
			"typescript": distresolver.New(repo, distresolver.NpmManifestSpec),
		},
	}
}

func (c *resolverCache) Resolve(language, file string) (distresolver.Distribution, bool) {
	r, ok := c.r[language]
	if !ok {
		return distresolver.Distribution{
			Name:   c.repo.FallbackName,
			Source: c.repo.FallbackSource,
		}, false
	}
	abs := file
	if !filepath.IsAbs(file) {
		abs = filepath.Join(c.repo.Root, file)
	}
	d, err := r.Resolve(abs)
	if err != nil {
		return distresolver.Distribution{
			Name:   c.repo.FallbackName,
			Source: c.repo.FallbackSource,
		}, false
	}
	return d, true
}

// normalizeOne builds a model.FunctionNode from a legacyFunction by
// re-resolving identity through Canonicalize. Returns a warning string
// (non-empty) if the legacy node could not be re-resolved cleanly and
// fell back to repo identity.
func normalizeOne(ln legacyFunction, resolvers *resolverCache) (model.FunctionNode, string, error) {
	dist, ok := resolvers.Resolve(ln.Language, ln.File)
	warning := ""
	if !ok || dist.Source != model.IdentityFromDistribution {
		warning = fmt.Sprintf("could not re-resolve %s (%s) — using %s", ln.QualifiedName, ln.File, dist.Source)
	}

	container := strings.TrimPrefix(ln.Receiver, "*")
	pointer := strings.HasPrefix(ln.Receiver, "*")

	parts := model.IdentityParts{
		Distribution:    dist.Name,
		ModulePath:      ln.Package,
		Container:       container,
		Name:            ln.Name,
		PointerReceiver: pointer,
	}
	base, err := parser.Mint(parts, ln.Language, ln.File, dist.Source)
	if err != nil {
		return model.FunctionNode{}, warning, err
	}
	return model.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            ln.StartLine,
			EndLine:              ln.EndLine,
			Exported:             ln.Exported,
			Tag:                  ln.Tag,
			Receiver:             ln.Receiver,
			PointerReceiver:      pointer,
			ContractCompleteness: ln.ContractCompleteness,
		},
	}, warning, nil
}
