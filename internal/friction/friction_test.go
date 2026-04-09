package friction

import (
	"context"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/query"
)

func makeRef(name string, source model.IdentitySource) query.FunctionRef {
	return query.FunctionRef{
		QualifiedName:  name,
		Kind:           model.KindFunction,
		Environment:    model.EnvironmentLocal,
		IdentitySource: source,
		Name:           name,
	}
}

func TestDetect_OmittedTools(t *testing.T) {
	d := New()
	for _, tool := range []string{"contract", "exports", "domains", "domain", "coupling", "graph_summary"} {
		if HasBoundary(tool) {
			t.Errorf("HasBoundary(%q) should be false", tool)
		}
		got := d.Detect(context.Background(), nil, tool, nil, query.PagedResult[query.FunctionRef]{
			Items: []query.FunctionRef{makeRef("x", model.IdentityFromRepoFallback)},
		})
		if got != nil {
			t.Errorf("Detect(%q) returned %d boundaries on omitted tool", tool, len(got))
		}
	}
}

func TestDetect_AffectedToolsHaveBoundary(t *testing.T) {
	for _, tool := range []string{"callers", "impact", "breaks", "test_coverage", "untested", "entry_points"} {
		if !HasBoundary(tool) {
			t.Errorf("HasBoundary(%q) should be true", tool)
		}
	}
	if !HasBoundary("subgraph") {
		t.Error("subgraph should have boundary")
	}
}

func TestDetect_FlagsRepoFallbackItems(t *testing.T) {
	d := New()
	pr := query.PagedResult[query.FunctionRef]{
		Items: []query.FunctionRef{
			makeRef("a", model.IdentityFromDistribution),
			makeRef("b", model.IdentityFromRepoFallback),
			makeRef("c", model.IdentityFromRepoFallbackNoRemote),
		},
	}
	got := d.Detect(context.Background(), nil, "callers", map[string]any{"function": "Target"}, pr)
	if len(got) != 2 {
		t.Fatalf("expected 2 boundaries (b, c), got %d", len(got))
	}
	for _, b := range got {
		if b.HostedResolution != HostedResolution {
			t.Errorf("hosted_resolution = %q", b.HostedResolution)
		}
		if b.Edge.From.QualifiedName != "Target" {
			t.Errorf("from = %q", b.Edge.From.QualifiedName)
		}
	}
}

func TestDetect_AllResolvedNoBoundaries(t *testing.T) {
	d := New()
	pr := query.PagedResult[query.FunctionRef]{
		Items: []query.FunctionRef{
			makeRef("a", model.IdentityFromDistribution),
			makeRef("b", model.IdentityFromDistribution),
		},
	}
	got := d.Detect(context.Background(), nil, "callers", map[string]any{"function": "Target"}, pr)
	if len(got) != 0 {
		t.Fatalf("expected 0 boundaries, got %d", len(got))
	}
}

func TestKindForTool(t *testing.T) {
	cases := map[string]string{
		"callers":       "external_caller",
		"impact":        "external_caller",
		"breaks":        "external_caller",
		"test_coverage": "external_test_caller",
		"subgraph":      "external_neighbor",
		"untested":      "untestable_external_caller",
		"entry_points":  "unverifiable_external_call",
	}
	for tool, want := range cases {
		if got := kindForTool(tool); got != want {
			t.Errorf("kindForTool(%q) = %q, want %q", tool, got, want)
		}
	}
}

func TestSubgraphBoundaries(t *testing.T) {
	d := New()
	sub := &query.SubgraphResult{
		Center: &query.FunctionDetail{FunctionRef: makeRef("center", model.IdentityFromDistribution)},
		Callers: []query.FunctionRef{
			makeRef("local-caller", model.IdentityFromDistribution),
			makeRef("ext-caller", model.IdentityFromRepoFallback),
		},
		Callees: []query.FunctionRef{
			makeRef("ext-callee", model.IdentityFromRepoFallback),
		},
	}
	got := d.Detect(context.Background(), nil, "subgraph", map[string]any{"function": "center"}, sub)
	if len(got) != 2 {
		t.Fatalf("expected 2 boundaries, got %d", len(got))
	}
}

func TestBoundarySignalConstant(t *testing.T) {
	if BoundarySignal != "hosted_resolves_cross_repo" {
		t.Fatalf("BoundarySignal frozen value changed")
	}
}
