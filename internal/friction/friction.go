// Package friction detects cross-repo / external boundaries in query
// results and emits structured `boundaries` annotations.
//
// At Rung 0, friction is the OSS install's only visible reminder that
// there's something the hosted store can do that the local store
// can't: resolve cross-repo references against a unified graph. The
// MCP layer wraps query responses for the seven affected tools with
// the boundaries the relevant detector reports.
//
// Per Q9, only seven tools surface boundaries:
//
//	callers, impact, breaks, test_coverage, subgraph, untested, entry_points
//
// The other six (contract, exports, domains, domain, coupling,
// graph_summary) intentionally omit the field — there's no cross-repo
// wall in their result shape.
package friction

import (
	"context"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/query"
	"github.com/codevalent/cvalent/internal/store"
)

// BoundarySignal is the fixed marker string returned alongside the
// boundaries array. Frozen at Rung 0; do not change.
const BoundarySignal = "hosted_resolves_cross_repo"

// HostedResolution is the fixed availability marker on every Boundary
// edge.
const HostedResolution = "available"

// Boundary describes one cross-repo / external reference observed in a
// query result. Matches the JSON shape from PRD § Q9.
type Boundary struct {
	Kind   string       `json:"kind"`
	Edge   BoundaryEdge `json:"edge"`
	Reason string       `json:"reason"`
	// HostedResolution is always "available" — Rung 1's hosted store
	// is the resolver.
	HostedResolution string `json:"hosted_resolution"`
}

// BoundaryEdge captures the (from, to_resolved) endpoints of a
// boundary-crossing reference. `from` is a node currently in the
// local graph; `to_resolved` is the Model B identity that the hosted
// store would resolve to.
type BoundaryEdge struct {
	From       BoundaryEndpoint `json:"from"`
	ToResolved BoundaryEndpoint `json:"to_resolved"`
}

// BoundaryEndpoint is a slim view of a node identity used inside
// Boundary entries.
type BoundaryEndpoint struct {
	QualifiedName  string               `json:"qualified_name"`
	IdentitySource model.IdentitySource `json:"identity_source"`
	Distribution   string               `json:"distribution,omitempty"`
}

// Detector inspects a query result and returns the boundaries that
// apply to it. Detection is keyed off the query name (so callers can
// route per-tool) and the local store (for "is the target resolved
// here?" checks).
type Detector interface {
	Detect(ctx context.Context, s *store.Store, queryName string, queryArgs map[string]any, result any) []Boundary
}

// New returns the default detector. The detector is per-tool aware
// internally; callers do not need to pick one.
func New() Detector { return &detector{} }

type detector struct{}

// hasBoundary returns true if a tool participates in the boundaries
// surface. Used by the MCP wrapper to decide whether to attach the
// envelope at all (the six unaffected tools must omit it entirely).
func HasBoundary(tool string) bool {
	switch tool {
	case "callers", "impact", "breaks", "test_coverage", "subgraph", "untested", "entry_points":
		return true
	}
	return false
}

// kindForTool returns the per-tool boundary kind enum from Q9.
func kindForTool(tool string) string {
	switch tool {
	case "callers":
		return "external_caller"
	case "impact":
		return "external_caller"
	case "breaks":
		return "external_caller"
	case "test_coverage":
		return "external_test_caller"
	case "subgraph":
		return "external_neighbor"
	case "untested":
		return "untestable_external_caller"
	case "entry_points":
		return "unverifiable_external_call"
	}
	return "external"
}

func (d *detector) Detect(ctx context.Context, s *store.Store, tool string, args map[string]any, result any) []Boundary {
	if !HasBoundary(tool) {
		return nil
	}
	switch tool {
	case "callers", "impact", "breaks":
		return d.detectListBoundaries(ctx, s, tool, args, result)
	case "test_coverage":
		return d.detectListBoundaries(ctx, s, tool, args, result)
	case "subgraph":
		return d.detectSubgraphBoundaries(ctx, s, args, result)
	case "untested", "entry_points":
		return d.detectListBoundaries(ctx, s, tool, args, result)
	}
	return nil
}

// detectListBoundaries scans a paged FunctionRef result for any item
// whose IdentitySource is something other than `distribution`. These
// are nodes the local store could not anchor against a manifest, and
// they are the canonical "boundary" — they're either external code or
// code that has no shippable distribution and would only resolve in a
// hosted environment with full repo metadata.
func (d *detector) detectListBoundaries(ctx context.Context, s *store.Store, tool string, args map[string]any, result any) []Boundary {
	pr, ok := result.(query.PagedResult[query.FunctionRef])
	if !ok {
		return nil
	}
	target := boundaryFromArgs(args)
	out := make([]Boundary, 0)
	for _, item := range pr.Items {
		if item.IdentitySource == model.IdentityFromDistribution {
			continue
		}
		out = append(out, Boundary{
			Kind: kindForTool(tool),
			Edge: BoundaryEdge{
				From: target,
				ToResolved: BoundaryEndpoint{
					QualifiedName:  item.QualifiedName,
					IdentitySource: item.IdentitySource,
					Distribution:   "", // unresolved by definition
				},
			},
			Reason:           "no manifest-resolved distribution; hosted resolver would join across repos",
			HostedResolution: HostedResolution,
		})
	}
	return out
}

// detectSubgraphBoundaries inspects a SubgraphResult for the same
// pattern, looking at both callers and callees.
func (d *detector) detectSubgraphBoundaries(ctx context.Context, s *store.Store, args map[string]any, result any) []Boundary {
	sr, ok := result.(*query.SubgraphResult)
	if !ok || sr == nil {
		return nil
	}
	target := boundaryFromArgs(args)
	out := make([]Boundary, 0)
	for _, item := range sr.Callers {
		if item.IdentitySource != model.IdentityFromDistribution {
			out = append(out, Boundary{
				Kind:             kindForTool("subgraph"),
				Edge:             BoundaryEdge{From: target, ToResolved: endpointFromRef(item)},
				Reason:           "subgraph caller has no manifest-resolved distribution",
				HostedResolution: HostedResolution,
			})
		}
	}
	for _, item := range sr.Callees {
		if item.IdentitySource != model.IdentityFromDistribution {
			out = append(out, Boundary{
				Kind:             kindForTool("subgraph"),
				Edge:             BoundaryEdge{From: target, ToResolved: endpointFromRef(item)},
				Reason:           "subgraph callee has no manifest-resolved distribution",
				HostedResolution: HostedResolution,
			})
		}
	}
	return out
}

func endpointFromRef(r query.FunctionRef) BoundaryEndpoint {
	return BoundaryEndpoint{
		QualifiedName:  r.QualifiedName,
		IdentitySource: r.IdentitySource,
	}
}

func boundaryFromArgs(args map[string]any) BoundaryEndpoint {
	if args == nil {
		return BoundaryEndpoint{}
	}
	name, _ := args["function"].(string)
	return BoundaryEndpoint{QualifiedName: name}
}
