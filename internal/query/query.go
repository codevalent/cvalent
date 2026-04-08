// Package query implements the 13 read-side query functions exposed by
// cvalent's CLI and MCP server.
//
// At Rung 0 every query reads from `*store.Store` (SQLite via
// modernc.org/sqlite). The legacy GoraphDB read path is gone — its
// last consumer is the migrator (internal/migrator), which is the only
// remaining importer of `internal/graph` and `goraphdb`.
//
// All function-level results carry the additive Q9 fields (`id`,
// `environment`, `identity_source`) on FunctionRef. The friction
// `boundaries` envelope lives in internal/friction and is wrapped onto
// responses by internal/mcp.
package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/store"
)

// QueryOpts controls pagination for query results.
type QueryOpts struct {
	Limit  int // 0 means unlimited
	Offset int
}

// DefaultOpts returns opts with Limit=50, Offset=0.
func DefaultOpts() QueryOpts { return QueryOpts{Limit: 50, Offset: 0} }

// UnlimitedOpts returns opts with no limit (for CLI usage).
func UnlimitedOpts() QueryOpts { return QueryOpts{Limit: 0, Offset: 0} }

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

// FunctionRef is the slim function shape used in list items. Q9 specifies
// which fields are present.
type FunctionRef struct {
	ID             uuid.UUID            `json:"id"`
	QualifiedName  string               `json:"qualified_name"`
	Kind           model.Kind           `json:"kind"`
	Environment    model.Environment    `json:"environment"`
	IdentitySource model.IdentitySource `json:"identity_source"`
	Name           string               `json:"name"`
	Module         string               `json:"package"` // historically called "package"
	File           string               `json:"file"`
	Language       string               `json:"language"`
}

// FunctionDetail is the full function shape used as the primary subject
// of `contract` and `subgraph`. Carries the five empty direction-signal
// arrays so that agents can write code that handles the empty and
// populated cases identically (Rungs 1+).
type FunctionDetail struct {
	FunctionRef
	StartLine            int            `json:"start_line"`
	EndLine              int            `json:"end_line"`
	Exported             bool           `json:"exported"`
	Tag                  string         `json:"tag"`
	Receiver             string         `json:"receiver,omitempty"`
	ContractCompleteness string         `json:"contract_completeness"`
	Contract             *ContractShape `json:"contract"`
	PipelineReferences   []any          `json:"pipeline_references"`
	RecentTraces         []any          `json:"recent_traces"`
	ContractHistory      []any          `json:"contract_history"`
	UpstreamStorage      []any          `json:"upstream_storage"`
	DownstreamStorage    []any          `json:"downstream_storage"`
}

// ContractShape is the parameter/return slot rendered into FunctionDetail.
type ContractShape struct {
	Parameters []model.Param    `json:"parameters"`
	Returns    model.ReturnSpec `json:"returns"`
}

func refFromNode(fn model.FunctionNode) FunctionRef {
	return FunctionRef{
		ID:             fn.ID,
		QualifiedName:  fn.QualifiedName,
		Kind:           fn.Kind,
		Environment:    fn.Environment,
		IdentitySource: fn.IdentitySource,
		Name:           fn.Name,
		Module:         fn.ModulePath,
		File:           fn.File,
		Language:       fn.Language,
	}
}

func detailFromNode(fn model.FunctionNode) FunctionDetail {
	return FunctionDetail{
		FunctionRef:          refFromNode(fn),
		StartLine:            fn.StartLine,
		EndLine:              fn.EndLine,
		Exported:             fn.Exported,
		Tag:                  fn.Tag,
		Receiver:             fn.Receiver,
		ContractCompleteness: fn.ContractCompleteness,
		Contract: &ContractShape{
			Parameters: fn.Params,
			Returns:    fn.Returns,
		},
		PipelineReferences: []any{},
		RecentTraces:       []any{},
		ContractHistory:    []any{},
		UpstreamStorage:    []any{},
		DownstreamStorage:  []any{},
	}
}

// ErrNotFound is returned by Contract / FindByName when no function
// matches the requested name.
var ErrNotFound = errors.New("query: function not found")

// FindByName resolves a fully-qualified or short name to a single
// FunctionNode. Falls back to Name match when QualifiedName misses.
func FindByName(ctx context.Context, s *store.Store, name string) (model.FunctionNode, error) {
	row := s.DB().QueryRowContext(ctx, `
		SELECT n.id FROM nodes n
		WHERE n.valid_until IS NULL
		  AND (n.qualified_name = ? OR n.name = ?)
		ORDER BY CASE WHEN n.qualified_name = ? THEN 0 ELSE 1 END
		LIMIT 1`, name, name, name)
	var idBytes []byte
	if err := row.Scan(&idBytes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.FunctionNode{}, ErrNotFound
		}
		return model.FunctionNode{}, err
	}
	id, err := uuid.FromBytes(idBytes)
	if err != nil {
		return model.FunctionNode{}, err
	}
	return s.GetNode(ctx, id)
}

// listFunctions reads every current Function node and returns the
// caller-friendly slice form. Caller paginates.
func listFunctions(ctx context.Context, s *store.Store) ([]model.FunctionNode, error) {
	return s.ListNodesByKind(ctx, model.KindFunction)
}

// listCallEdges returns every current call edge.
func listCallEdges(ctx context.Context, s *store.Store) ([]store.Edge, error) {
	return s.ListEdgesByKind(ctx, "call")
}

// callerIndex builds a from→[]to map keyed by node ID using the call
// edge table. Used by every reverse-traversal query (Callers, Impact,
// EntryPoints, Untested, TestCoverage).
func callerIndex(edges []store.Edge) (in, out map[uuid.UUID][]uuid.UUID) {
	in = map[uuid.UUID][]uuid.UUID{}
	out = map[uuid.UUID][]uuid.UUID{}
	for _, e := range edges {
		in[e.To] = append(in[e.To], e.From)
		out[e.From] = append(out[e.From], e.To)
	}
	return in, out
}

// Callers returns functions that call the named function.
func Callers(ctx context.Context, s *store.Store, name string, opts QueryOpts) (PagedResult[FunctionRef], error) {
	target, err := FindByName(ctx, s, name)
	if err != nil {
		return PagedResult[FunctionRef]{}, fmt.Errorf("%w: %s", err, name)
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	in, _ := callerIndex(edges)
	callerIDs := in[target.ID]
	refs := make([]FunctionRef, 0, len(callerIDs))
	for _, id := range callerIDs {
		fn, err := s.GetNode(ctx, id)
		if err != nil {
			continue
		}
		refs = append(refs, refFromNode(fn))
	}
	return paginate(refs, opts), nil
}

// Contract returns the FunctionDetail for the named function.
func Contract(ctx context.Context, s *store.Store, name string) (*FunctionDetail, error) {
	fn, err := FindByName(ctx, s, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, name)
	}
	d := detailFromNode(fn)
	return &d, nil
}

// Impact returns functions affected by changing the named function (up to depth).
func Impact(ctx context.Context, s *store.Store, name string, depth int, opts QueryOpts) (PagedResult[FunctionRef], error) {
	if depth <= 0 {
		depth = 3
	}
	target, err := FindByName(ctx, s, name)
	if err != nil {
		return PagedResult[FunctionRef]{}, fmt.Errorf("%w: %s", err, name)
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	in, _ := callerIndex(edges)

	// BFS upward through the in-edge index.
	visited := map[uuid.UUID]bool{target.ID: true}
	frontier := []uuid.UUID{target.ID}
	var collected []uuid.UUID
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		var next []uuid.UUID
		for _, id := range frontier {
			for _, p := range in[id] {
				if visited[p] {
					continue
				}
				visited[p] = true
				collected = append(collected, p)
				next = append(next, p)
			}
		}
		frontier = next
	}
	refs := make([]FunctionRef, 0, len(collected))
	for _, id := range collected {
		fn, err := s.GetNode(ctx, id)
		if err == nil {
			refs = append(refs, refFromNode(fn))
		}
	}
	return paginate(refs, opts), nil
}

// Breaks returns callers whose argument shape mismatches the function
// contract. Rung 0 reuses Callers semantics — the contract-mismatch
// detection is a Rung 4 feature.
func Breaks(ctx context.Context, s *store.Store, name string, opts QueryOpts) (PagedResult[FunctionRef], error) {
	return Callers(ctx, s, name, opts)
}

// EntryPoints returns functions with no incoming call edges.
func EntryPoints(ctx context.Context, s *store.Store, opts QueryOpts) (PagedResult[FunctionRef], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	in, _ := callerIndex(edges)
	refs := make([]FunctionRef, 0, len(all))
	for _, fn := range all {
		if len(in[fn.ID]) == 0 {
			refs = append(refs, refFromNode(fn))
		}
	}
	return paginate(refs, opts), nil
}

// Exports returns exported (public) functions in the named module.
func Exports(ctx context.Context, s *store.Store, module string, opts QueryOpts) (PagedResult[FunctionRef], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	refs := make([]FunctionRef, 0)
	for _, fn := range all {
		if fn.ModulePath != module {
			continue
		}
		if !fn.Exported || fn.Tag == "test" {
			continue
		}
		refs = append(refs, refFromNode(fn))
	}
	return paginate(refs, opts), nil
}

// DomainInfo represents a module grouping.
type DomainInfo struct {
	Module    string `json:"module"`
	Functions int    `json:"functions"`
}

// Domains returns all distinct modules with function counts.
func Domains(ctx context.Context, s *store.Store, opts QueryOpts) (PagedResult[DomainInfo], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[DomainInfo]{}, err
	}
	counts := map[string]int{}
	for _, fn := range all {
		if fn.ModulePath == "" {
			continue
		}
		counts[fn.ModulePath]++
	}
	infos := make([]DomainInfo, 0, len(counts))
	for mod, count := range counts {
		infos = append(infos, DomainInfo{Module: mod, Functions: count})
	}
	return paginate(infos, opts), nil
}

// Domain returns functions within a specific module.
func Domain(ctx context.Context, s *store.Store, module string, opts QueryOpts) (PagedResult[FunctionRef], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	refs := make([]FunctionRef, 0)
	for _, fn := range all {
		if fn.ModulePath == module {
			refs = append(refs, refFromNode(fn))
		}
	}
	return paginate(refs, opts), nil
}

// CouplingInfo represents cross-module dependency density.
type CouplingInfo struct {
	FromModule string `json:"from_module"`
	ToModule   string `json:"to_module"`
	EdgeCount  int    `json:"edge_count"`
}

// Coupling returns cross-module dependency density.
func Coupling(ctx context.Context, s *store.Store, opts QueryOpts) (PagedResult[CouplingInfo], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[CouplingInfo]{}, err
	}
	moduleByID := map[uuid.UUID]string{}
	for _, fn := range all {
		moduleByID[fn.ID] = fn.ModulePath
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[CouplingInfo]{}, err
	}
	pairs := map[string]*CouplingInfo{}
	for _, e := range edges {
		from := moduleByID[e.From]
		to := moduleByID[e.To]
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "→" + to
		ci, ok := pairs[key]
		if !ok {
			ci = &CouplingInfo{FromModule: from, ToModule: to}
			pairs[key] = ci
		}
		ci.EdgeCount++
	}
	out := make([]CouplingInfo, 0, len(pairs))
	for _, ci := range pairs {
		out = append(out, *ci)
	}
	return paginate(out, opts), nil
}

// Untested returns application functions with no incoming test edges.
func Untested(ctx context.Context, s *store.Store, opts QueryOpts) (PagedResult[FunctionRef], error) {
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	tagByID := map[uuid.UUID]string{}
	for _, fn := range all {
		tagByID[fn.ID] = fn.Tag
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	in, _ := callerIndex(edges)
	refs := make([]FunctionRef, 0)
	for _, fn := range all {
		if fn.Tag == "test" {
			continue
		}
		hasTestCaller := false
		for _, callerID := range in[fn.ID] {
			if tagByID[callerID] == "test" {
				hasTestCaller = true
				break
			}
		}
		if !hasTestCaller {
			refs = append(refs, refFromNode(fn))
		}
	}
	return paginate(refs, opts), nil
}

// TestCoverage returns test functions that exercise the named function.
func TestCoverage(ctx context.Context, s *store.Store, name string, opts QueryOpts) (PagedResult[FunctionRef], error) {
	target, err := FindByName(ctx, s, name)
	if err != nil {
		return PagedResult[FunctionRef]{}, fmt.Errorf("%w: %s", err, name)
	}
	all, err := listFunctions(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	tagByID := map[uuid.UUID]string{}
	for _, fn := range all {
		tagByID[fn.ID] = fn.Tag
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return PagedResult[FunctionRef]{}, err
	}
	in, _ := callerIndex(edges)
	refs := make([]FunctionRef, 0)
	for _, callerID := range in[target.ID] {
		if tagByID[callerID] != "test" {
			continue
		}
		fn, err := s.GetNode(ctx, callerID)
		if err == nil {
			refs = append(refs, refFromNode(fn))
		}
	}
	return paginate(refs, opts), nil
}

// SummaryResult is the response shape for graph_summary.
type SummaryResult struct {
	Modules        []DomainInfo `json:"modules"`
	TotalFunctions int          `json:"total_functions"`
	TotalEdges     int          `json:"total_edges"`
	UntestedCount  int          `json:"untested_count"`
}

// GraphSummary returns a compact overview of the graph.
func GraphSummary(ctx context.Context, s *store.Store) (*SummaryResult, error) {
	domains, err := Domains(ctx, s, UnlimitedOpts())
	if err != nil {
		return nil, err
	}
	untested, err := Untested(ctx, s, UnlimitedOpts())
	if err != nil {
		return nil, err
	}
	edges, err := listCallEdges(ctx, s)
	if err != nil {
		return nil, err
	}
	totalFn := 0
	for _, d := range domains.Items {
		totalFn += d.Functions
	}
	return &SummaryResult{
		Modules:        domains.Items,
		TotalFunctions: totalFn,
		TotalEdges:     len(edges),
		UntestedCount:  untested.TotalCount,
	}, nil
}

// SubgraphResult is the response shape for `subgraph`.
type SubgraphResult struct {
	Center  *FunctionDetail `json:"center"`
	Callers []FunctionRef   `json:"callers"`
	Callees []FunctionRef   `json:"callees"`
	Hops    int             `json:"hops"`
}

// Subgraph extracts the N-hop neighborhood of a function.
func Subgraph(ctx context.Context, s *store.Store, name string, hops int) (*SubgraphResult, error) {
	if hops <= 0 {
		hops = 2
	}
	center, err := Contract(ctx, s, name)
	if err != nil {
		return nil, err
	}
	callers, err := Callers(ctx, s, name, UnlimitedOpts())
	if err != nil {
		return nil, err
	}
	callees, err := Impact(ctx, s, name, hops, UnlimitedOpts())
	if err != nil {
		return nil, err
	}
	return &SubgraphResult{
		Center:  center,
		Callers: callers.Items,
		Callees: callees.Items,
		Hops:    hops,
	}, nil
}

// FormatRefs formats a list of FunctionRef for terminal output.
func FormatRefs(refs []FunctionRef) string {
	if len(refs) == 0 {
		return "No results.\n"
	}
	var sb strings.Builder
	for _, r := range refs {
		sb.WriteString("  ")
		sb.WriteString(r.QualifiedName)
		sb.WriteString("\n    ")
		sb.WriteString(r.File)
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatDetail renders a FunctionDetail for terminal output.
func FormatDetail(d *FunctionDetail) string {
	if d == nil {
		return "(none)\n"
	}
	body, _ := json.MarshalIndent(d, "", "  ")
	return string(body) + "\n"
}
