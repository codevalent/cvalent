package build

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	graphdb "github.com/mstrYoda/goraphdb"

	"github.com/codevalent/cvalent/internal/config"
	"github.com/codevalent/cvalent/internal/graph"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
	goparser "github.com/codevalent/cvalent/internal/parser/golang"
	javaparser "github.com/codevalent/cvalent/internal/parser/java"
	pyparser "github.com/codevalent/cvalent/internal/parser/python"
	tsparser "github.com/codevalent/cvalent/internal/parser/typescript"
	"github.com/codevalent/cvalent/internal/resolver"
	"github.com/codevalent/cvalent/internal/walker"
)

// languageManifest maps a parser language identifier to the manifest
// spec used to resolve its distribution. New languages add an entry
// here when their parser lands.
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
	ContractCoverage map[string]int // completeness -> count
	BuildTime        time.Duration
	GraphPath        string
}

// Options controls the build behavior.
type Options struct {
	Root      string
	ScopePath string // empty = full build
}

// Run executes the full build pipeline: config -> walk -> parse -> graph.
func Run(opts Options) (*Result, error) {
	start := time.Now()

	// Auto-init if needed
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

	// Walk
	walkResult, err := walker.Walk(opts.Root, cfg.Exclude, opts.ScopePath)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	// Build parsers
	parsers := map[string]parser.LanguageParser{
		"go":         goparser.New(),
		"java":       javaparser.New(),
		"typescript": tsparser.New(),
		"python":     pyparser.New(),
	}

	// Per-language Run state: one Resolver per language, all sharing the
	// same RepoContext (same git root). Identity minting flows through
	// distresolver per-file.
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

	// Parse all files
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
			// Stamp the relative path on the node so downstream
			// presenters render the same paths users see in their tree.
			for i := range nodes {
				nodes[i].File = file
			}
			allNodes = append(allNodes, nodes...)
			fileCount++
		}
	}

	// Build graph
	graphPath := config.GraphPath(opts.Root)
	// Remove old graph for full rebuild
	os.Remove(graphPath)
	os.MkdirAll(filepath.Dir(graphPath), 0755)

	g, err := graph.Open(graphPath)
	if err != nil {
		return nil, fmt.Errorf("open graph: %w", err)
	}
	defer g.Close()

	if err := g.CreateSchema(); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Insert function nodes and build qualified_name -> nodeID map
	contractCoverage := map[string]int{}
	nodeIDMap := map[string]graphdb.NodeID{}
	for _, node := range allNodes {
		props := nodeToProps(node)
		var id graphdb.NodeID
		if node.Tag == "test" {
			id, err = g.AddTestFunction(props)
		} else {
			id, err = g.AddFunction(props)
		}
		if err != nil {
			return nil, fmt.Errorf("add function %s: %w", node.QualifiedName, err)
		}
		nodeIDMap[node.QualifiedName] = id
		contractCoverage[node.ContractCompleteness]++
	}

	// Resolve cross-file edges
	callEdges, err := resolver.Resolve(opts.Root, allNodes)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	edgeCount := 0
	for _, edge := range callEdges {
		fromID, fromOK := nodeIDMap[edge.CallerQualified]
		toID, toOK := nodeIDMap[edge.CalleeQualified]
		if fromOK && toOK {
			_, err := g.AddCallEdge(fromID, toID, nil)
			if err != nil {
				continue
			}
			edgeCount++
		}
	}

	// Add GraphMeta
	var langs []string
	for lang := range walkResult.Files {
		langs = append(langs, lang)
	}
	g.AddGraphMeta(graphdb.Props{
		"schema_version":  float64(1),
		"cvalent_version": "0.1.0-dev",
		"build_time":      time.Now().Format(time.RFC3339),
		"function_count":  float64(len(allNodes)),
		"file_count":      float64(fileCount),
		"languages":       fmt.Sprintf("%v", langs),
	})

	// Rebuild indexes after bulk insert
	g.CreateSchema()

	return &Result{
		FunctionCount:    len(allNodes),
		EdgeCount:        edgeCount,
		FileCount:        fileCount,
		Languages:        langs,
		Skipped:          walkResult.Skipped,
		ContractCoverage: contractCoverage,
		BuildTime:        time.Since(start),
		GraphPath:        graphPath,
	}, nil
}

func nodeToProps(node parser.FunctionNode) graphdb.Props {
	module := filepath.Dir(node.File)
	props := graphdb.Props{
		"name":                  node.Name,
		"qualified_name":        node.QualifiedName,
		"file":                  node.File,
		"package":               node.ModulePath,
		"module":                module,
		"language":              node.Language,
		"start_line":            float64(node.StartLine),
		"end_line":              float64(node.EndLine),
		"kind":                  node.Kind,
		"exported":              node.Exported,
		"tag":                   node.Tag,
		"contract_completeness": node.ContractCompleteness,
	}
	if node.Receiver != "" {
		props["receiver"] = node.Receiver
	}
	return props
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

	s += fmt.Sprintf("  Graph:     %s\n", r.GraphPath)
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
