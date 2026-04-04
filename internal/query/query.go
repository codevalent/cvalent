package query

import (
	"fmt"
	"strings"

	graphdb "github.com/mstrYoda/goraphdb"

	"github.com/codevalent/cvalent/internal/graph"
)

// QueryOpts controls pagination for query results.
type QueryOpts struct {
	Limit  int // 0 means unlimited
	Offset int
}

// DefaultOpts returns opts with Limit=50, Offset=0.
func DefaultOpts() QueryOpts {
	return QueryOpts{Limit: 50, Offset: 0}
}

// UnlimitedOpts returns opts with no limit (for CLI usage).
func UnlimitedOpts() QueryOpts {
	return QueryOpts{Limit: 0, Offset: 0}
}

// PagedResult wraps a slice of results with pagination metadata.
type PagedResult[T any] struct {
	Items      []T  `json:"items"`
	TotalCount int  `json:"total_count"`
	Returned   int  `json:"returned"`
	Offset     int  `json:"offset"`
	Truncated  bool `json:"truncated"`
}

func paginate[T any](all []T, opts QueryOpts) PagedResult[T] {
	total := len(all)
	start := opts.Offset
	if start > total {
		start = total
	}
	remaining := all[start:]
	if opts.Limit > 0 && len(remaining) > opts.Limit {
		remaining = remaining[:opts.Limit]
	}
	return PagedResult[T]{
		Items:      remaining,
		TotalCount: total,
		Returned:   len(remaining),
		Offset:     start,
		Truncated:  start+len(remaining) < total,
	}
}

// FunctionInfo is a simplified view of a function node for query results.
type FunctionInfo struct {
	Name          string
	QualifiedName string
	File          string
	Module        string
	StartLine     int
	EndLine       int
	Exported      bool
	Tag           string
	Contract      string
	Completeness  string
}

func nodeToInfo(n *graphdb.Node) FunctionInfo {
	return FunctionInfo{
		Name:          n.GetString("name"),
		QualifiedName: n.GetString("qualified_name"),
		File:          n.GetString("file"),
		Module:        n.GetString("module"),
		StartLine:     int(n.GetFloat("start_line")),
		EndLine:       int(n.GetFloat("end_line")),
		Exported:      n.Props["exported"] == true,
		Tag:           n.GetString("tag"),
		Contract:      n.GetString("contract"),
		Completeness:  n.GetString("contract_completeness"),
	}
}

// Callers returns functions that call the named function.
func Callers(g *graph.Graph, name string, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	node, err := g.FindByName(name)
	if err != nil || node == nil {
		return PagedResult[FunctionInfo]{}, fmt.Errorf("function %q not found", name)
	}
	callers, err := g.Callers(node.ID)
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, n := range callers {
		all = append(all, nodeToInfo(n))
	}
	return paginate(all, opts), nil
}

// Contract returns the contract of the named function.
func Contract(g *graph.Graph, name string) (*FunctionInfo, error) {
	result, err := g.Query(fmt.Sprintf(`MATCH (f:Function {name: "%s"}) RETURN f`, name))
	if err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("function %q not found", name)
	}
	node := result.Rows[0]["f"].(*graphdb.Node)
	info := nodeToInfo(node)
	return &info, nil
}

// Impact returns functions affected by changing the named function (up to depth).
func Impact(g *graph.Graph, name string, depth int, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	if depth <= 0 {
		depth = 3
	}
	query := fmt.Sprintf(
		`MATCH (caller:Function)-[:CALLS*1..%d]->(f:Function {name: "%s"}) RETURN caller`,
		depth, name)
	result, err := g.Query(query)
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	seen := map[string]bool{}
	for _, row := range result.Rows {
		n := row["caller"].(*graphdb.Node)
		qn := n.GetString("qualified_name")
		if !seen[qn] {
			seen[qn] = true
			all = append(all, nodeToInfo(n))
		}
	}
	return paginate(all, opts), nil
}

// Breaks returns callers of a function with their edge data shapes.
func Breaks(g *graph.Graph, name string, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	return Callers(g, name, opts)
}

// EntryPoints returns functions with no incoming CALLS edges.
func EntryPoints(g *graph.Graph, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	result, err := g.Query(`MATCH (f:Function) RETURN f`)
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, row := range result.Rows {
		n := row["f"].(*graphdb.Node)
		callers, _ := g.Callers(n.ID)
		if len(callers) == 0 {
			all = append(all, nodeToInfo(n))
		}
	}
	return paginate(all, opts), nil
}

// Exports returns exported functions in the named module.
func Exports(g *graph.Graph, module string, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	result, err := g.Query(fmt.Sprintf(
		`MATCH (f:Function {module: "%s"}) WHERE f.exported = true RETURN f`, module))
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, row := range result.Rows {
		n := row["f"].(*graphdb.Node)
		if n.Props["is_test"] != true {
			all = append(all, nodeToInfo(n))
		}
	}
	return paginate(all, opts), nil
}

// DomainInfo represents a module grouping.
type DomainInfo struct {
	Module    string
	Functions int
	Edges     int
}

// Domains returns all distinct modules with function counts.
func Domains(g *graph.Graph, opts QueryOpts) (PagedResult[DomainInfo], error) {
	result, err := g.Query(`MATCH (f:Function) RETURN f.module`)
	if err != nil {
		return PagedResult[DomainInfo]{}, err
	}
	counts := map[string]int{}
	for _, row := range result.Rows {
		if mod, ok := row["f.module"].(string); ok && mod != "" {
			counts[mod]++
		}
	}
	var all []DomainInfo
	for mod, count := range counts {
		all = append(all, DomainInfo{Module: mod, Functions: count})
	}
	return paginate(all, opts), nil
}

// Domain returns functions within a specific module.
func Domain(g *graph.Graph, module string, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	result, err := g.Query(fmt.Sprintf(
		`MATCH (f:Function {module: "%s"}) RETURN f`, module))
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, row := range result.Rows {
		all = append(all, nodeToInfo(row["f"].(*graphdb.Node)))
	}
	return paginate(all, opts), nil
}

// CouplingInfo represents cross-module dependency.
type CouplingInfo struct {
	FromModule string
	ToModule   string
	EdgeCount  int
}

// Coupling returns cross-module dependency density.
func Coupling(g *graph.Graph, opts QueryOpts) (PagedResult[CouplingInfo], error) {
	result, err := g.Query(`MATCH (a:Function)-[:CALLS]->(b:Function) RETURN a.module, b.module`)
	if err != nil {
		return PagedResult[CouplingInfo]{}, err
	}
	pairs := map[string]*CouplingInfo{}
	for _, row := range result.Rows {
		amod, _ := row["a.module"].(string)
		bmod, _ := row["b.module"].(string)
		if amod == bmod || amod == "" || bmod == "" {
			continue
		}
		key := amod + " -> " + bmod
		if _, ok := pairs[key]; !ok {
			pairs[key] = &CouplingInfo{FromModule: amod, ToModule: bmod}
		}
		pairs[key].EdgeCount++
	}
	var all []CouplingInfo
	for _, ci := range pairs {
		all = append(all, *ci)
	}
	return paginate(all, opts), nil
}

// Untested returns application functions with no incoming test edges.
func Untested(g *graph.Graph, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	result, err := g.Query(`MATCH (f:Function) RETURN f`)
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, row := range result.Rows {
		n := row["f"].(*graphdb.Node)
		if n.Props["is_test"] == true {
			continue
		}
		callers, _ := g.Callers(n.ID)
		hasTestCaller := false
		for _, caller := range callers {
			if caller.Props["is_test"] == true {
				hasTestCaller = true
				break
			}
		}
		if !hasTestCaller {
			all = append(all, nodeToInfo(n))
		}
	}
	return paginate(all, opts), nil
}

// TestCoverage returns test functions that exercise the named function.
func TestCoverage(g *graph.Graph, name string, opts QueryOpts) (PagedResult[FunctionInfo], error) {
	node, err := g.FindByName(name)
	if err != nil || node == nil {
		return PagedResult[FunctionInfo]{}, fmt.Errorf("function %q not found", name)
	}
	callers, err := g.Callers(node.ID)
	if err != nil {
		return PagedResult[FunctionInfo]{}, err
	}
	var all []FunctionInfo
	for _, n := range callers {
		if n.Props["is_test"] == true {
			all = append(all, nodeToInfo(n))
		}
	}
	return paginate(all, opts), nil
}

// FormatInfo formats a list of FunctionInfo for terminal output.
func FormatInfo(infos []FunctionInfo) string {
	if len(infos) == 0 {
		return "No results.\n"
	}
	var sb strings.Builder
	for _, info := range infos {
		sb.WriteString(fmt.Sprintf("  %s\n    %s:%d-%d  [%s]\n",
			info.QualifiedName, info.File, info.StartLine, info.EndLine, info.Completeness))
	}
	return sb.String()
}
