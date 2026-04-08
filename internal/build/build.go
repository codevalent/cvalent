// Package build orchestrates the parser → store pipeline. It reads
// the per-repo config, walks the filesystem via internal/walker,
// dispatches each file to the matching language parser, mints
// identities through internal/parser/distresolver + internal/model,
// and writes the resulting nodes and call edges into the SQLite store.
//
// At Rung 0 the build is idempotent: running twice produces the same
// store state because UpsertNode/UpsertEdge use INSERT OR REPLACE on
// (id, valid_from = epoch).
package build

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/codevalent/cvalent/internal/config"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
	goparser "github.com/codevalent/cvalent/internal/parser/golang"
	javaparser "github.com/codevalent/cvalent/internal/parser/java"
	pyparser "github.com/codevalent/cvalent/internal/parser/python"
	tsparser "github.com/codevalent/cvalent/internal/parser/typescript"
	"github.com/codevalent/cvalent/internal/resolver"
	"github.com/codevalent/cvalent/internal/store"
	"github.com/codevalent/cvalent/internal/walker"
)

// languageManifest maps a parser language identifier to the manifest
// spec used to resolve its distribution.
var languageManifest = map[string]distresolver.ManifestSpec{
	"go":         distresolver.GoManifestSpec,
	"java":       distresolver.JavaManifestSpec,
	"typescript": distresolver.NpmManifestSpec,
	"python":     distresolver.PythonManifestSpec,
}

// Result contains build output statistics.
type Result struct {
	FunctionCount    int
	EdgeCount        int
	FileCount        int
	Languages        []string
	Skipped          map[string]int
	ContractCoverage map[string]int
	BuildTime        time.Duration
	StorePath        string
}

// Options controls the build behavior.
type Options struct {
	Root      string
	ScopePath string // empty = full build
}

// Run executes the full build pipeline: config → walk → parse → store.
func Run(opts Options) (*Result, error) {
	ctx := context.Background()
	start := time.Now()

	// Auto-init if needed.
	var cfg *config.Config
	if config.Exists(opts.Root) {
		var err error
		cfg, err = config.Load(opts.Root)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	} else {
		var err error
		cfg, err = config.Init(opts.Root)
		if err != nil {
			return nil, fmt.Errorf("init: %w", err)
		}
	}

	walkResult, err := walker.Walk(opts.Root, cfg.Exclude, opts.ScopePath)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	parsers := map[string]parser.LanguageParser{
		"go":         goparser.New(),
		"java":       javaparser.New(),
		"typescript": tsparser.New(),
		"python":     pyparser.New(),
	}

	repoCtx, err := distresolver.NewRepoContext(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("repo context: %w", err)
	}
	runs := make(map[string]*parser.Run, len(parsers))
	for lang := range parsers {
		spec, ok := languageManifest[lang]
		if !ok {
			continue
		}
		runs[lang] = &parser.Run{
			Resolver: distresolver.New(repoCtx, spec),
			Repo:     repoCtx,
		}
	}

	var allNodes []parser.FunctionNode
	fileCount := 0
	for lang, files := range walkResult.Files {
		p, ok := parsers[lang]
		if !ok {
			continue
		}
		run := runs[lang]
		for _, file := range files {
			fullPath := filepath.Join(opts.Root, file)
			source, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			nodes, err := p.Parse(run, fullPath, source)
			if err != nil {
				continue
			}
			for i := range nodes {
				nodes[i].File = file
			}
			allNodes = append(allNodes, nodes...)
			fileCount++
		}
	}

	// Open the destination store. We start from a fresh file so that
	// every build is a full rebuild — the parity harness depends on
	// reproducibility, not on incremental updates.
	storePath := config.StorePath(opts.Root)
	_ = os.Remove(storePath)
	_ = os.MkdirAll(filepath.Dir(storePath), 0o755)

	s, err := store.Open(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	contractCoverage := map[string]int{}
	for _, n := range allNodes {
		contractCoverage[n.ContractCompleteness]++
	}
	if err := s.UpsertNodes(ctx, allNodes); err != nil {
		return nil, fmt.Errorf("upsert nodes: %w", err)
	}

	// Resolve cross-file call edges and write them.
	callEdges, err := resolver.Resolve(opts.Root, allNodes)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	idByQual := map[string]uuid.UUID{}
	for _, n := range allNodes {
		idByQual[n.QualifiedName] = n.ID
	}

	edgeCount := 0
	for _, e := range callEdges {
		from, fok := idByQual[e.CallerQualified]
		to, tok := idByQual[e.CalleeQualified]
		if !fok || !tok {
			continue
		}
		edgeID := edgeUUID(from, to)
		if err := s.UpsertEdge(ctx, store.Edge{
			ID:   edgeID,
			From: from,
			To:   to,
			Kind: "call",
		}); err != nil {
			continue
		}
		edgeCount++
	}

	var langs []string
	for lang := range walkResult.Files {
		langs = append(langs, lang)
	}

	return &Result{
		FunctionCount:    len(allNodes),
		EdgeCount:        edgeCount,
		FileCount:        fileCount,
		Languages:        langs,
		Skipped:          walkResult.Skipped,
		ContractCoverage: contractCoverage,
		BuildTime:        time.Since(start),
		StorePath:        storePath,
	}, nil
}

// edgeUUID derives a deterministic UUIDv5-style identifier from the
// (from, to) pair so that re-runs of the build produce the same edge
// rows. We use a SHA-1 truncated to 16 bytes (the same width as a
// UUID) and stamp the version/variant bits manually.
func edgeUUID(from, to uuid.UUID) uuid.UUID {
	var buf [33]byte
	copy(buf[:16], from[:])
	copy(buf[16:32], to[:])
	buf[32] = 'c' // namespace separator: call edge
	sum := sha1.Sum(buf[:])
	var id uuid.UUID
	copy(id[:], sum[:16])
	// Set version 5 bits (0101) and RFC 4122 variant.
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	_ = binary.BigEndian
	return id
}

// FormatSummary formats a human-readable build summary.
func FormatSummary(r *Result) string {
	s := fmt.Sprintf("Build complete in %v\n", r.BuildTime.Round(time.Millisecond))
	s += fmt.Sprintf("  Files:     %d\n", r.FileCount)
	s += fmt.Sprintf("  Functions: %d\n", r.FunctionCount)
	s += fmt.Sprintf("  Edges:     %d\n", r.EdgeCount)

	if len(r.ContractCoverage) > 0 {
		s += "  Contracts: "
		first := true
		for comp, count := range r.ContractCoverage {
			if !first {
				s += ", "
			}
			s += fmt.Sprintf("%d %s", count, comp)
			first = false
		}
		s += "\n"
	}

	for ext, count := range r.Skipped {
		lang := skippedLanguageName(ext)
		s += fmt.Sprintf("  Skipped %d %s files (%s support coming)\n", count, ext, lang)
	}

	s += fmt.Sprintf("  Store:     %s\n", r.StorePath)
	return s
}

func skippedLanguageName(ext string) string {
	switch ext {
	case ".rb":
		return "Ruby"
	case ".rs":
		return "Rust"
	case ".cs":
		return "C#"
	case ".kt":
		return "Kotlin"
	case ".swift":
		return "Swift"
	default:
		return ext
	}
}
