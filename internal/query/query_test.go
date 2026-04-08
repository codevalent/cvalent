package query

import (
	"context"
	"strings"
	"testing"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/store"
)

func mintFn(t *testing.T, dist, modulePath, name string, isTest bool) model.FunctionNode {
	t.Helper()
	parts := model.IdentityParts{
		Distribution: dist,
		ModulePath:   modulePath,
		Name:         name,
	}
	id, canon, err := model.MintFunctionID(model.EnvironmentLocal, parts)
	if err != nil {
		t.Fatal(err)
	}
	tag := "application"
	if isTest {
		tag = "test"
	}
	return model.FunctionNode{
		Node: model.Node{
			ID:             id,
			Environment:    model.EnvironmentLocal,
			Kind:           model.KindFunction,
			QualifiedName:  canon.QualifiedName(),
			Name:           name,
			Distribution:   dist,
			ModulePath:     modulePath,
			Language:       "go",
			File:           modulePath + "/" + strings.ToLower(name) + ".go",
			IdentitySource: model.IdentityFromDistribution,
		},
		FunctionMeta: model.FunctionMeta{
			StartLine:            1,
			EndLine:              5,
			Exported:             true,
			Tag:                  tag,
			ContractCompleteness: "full",
			Params:               []model.Param{{Name: "x", Type: "int"}},
			Returns: model.ReturnSpec{
				Values: []model.ReturnValue{{Type: "error", Nullable: true}},
			},
		},
	}
}

func edgeID(a, b [16]byte) [16]byte {
	var out [16]byte
	for i := 0; i < 16; i++ {
		out[i] = a[i] ^ b[i] ^ 0x42
	}
	out[6] = (out[6] & 0x0f) | 0x50
	out[8] = (out[8] & 0x3f) | 0x80
	return out
}

func openFixture(t *testing.T) (*store.Store, map[string]model.FunctionNode) {
	t.Helper()
	s, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	main := mintFn(t, "example.com/x", "cmd", "main", false)
	process := mintFn(t, "example.com/x", "order", "ProcessOrder", false)
	validate := mintFn(t, "example.com/x", "order", "validate", false)
	validate.Exported = false
	save := mintFn(t, "example.com/x", "store", "saveOrder", false)
	save.Exported = false
	test := mintFn(t, "example.com/x", "order", "TestProcessOrder", true)

	all := map[string]model.FunctionNode{
		"main": main, "ProcessOrder": process, "validate": validate, "saveOrder": save, "TestProcessOrder": test,
	}
	for _, fn := range all {
		if err := s.UpsertNode(ctx, fn); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []store.Edge{
		{ID: edgeID(main.ID, process.ID), From: main.ID, To: process.ID, Kind: "call"},
		{ID: edgeID(process.ID, validate.ID), From: process.ID, To: validate.ID, Kind: "call"},
		{ID: edgeID(process.ID, save.ID), From: process.ID, To: save.ID, Kind: "call"},
		{ID: edgeID(test.ID, process.ID), From: test.ID, To: process.ID, Kind: "call"},
	} {
		if err := s.UpsertEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	return s, all
}

func ctxBg() context.Context { return context.Background() }

func TestCallers(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Callers(ctxBg(), s, "ProcessOrder", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 2 {
		t.Fatalf("want 2 callers, got %d", len(r.Items))
	}
}

func TestContract_AddsDirectionSignals(t *testing.T) {
	s, _ := openFixture(t)
	d, err := Contract(ctxBg(), s, "ProcessOrder")
	if err != nil {
		t.Fatal(err)
	}
	if d.Contract == nil {
		t.Fatalf("contract nil")
	}
	if d.PipelineReferences == nil || d.RecentTraces == nil {
		t.Fatalf("direction signals must be non-nil empty arrays")
	}
}

func TestImpact(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Impact(ctxBg(), s, "validate", 3, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) < 1 {
		t.Fatalf("want at least 1, got %d", len(r.Items))
	}
}

func TestEntryPoints(t *testing.T) {
	s, _ := openFixture(t)
	r, err := EntryPoints(ctxBg(), s, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// main and TestProcessOrder have no callers
	if len(r.Items) != 2 {
		t.Fatalf("want 2 entry points, got %d", len(r.Items))
	}
}

func TestExports(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Exports(ctxBg(), s, "order", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// ProcessOrder is exported, validate is not, TestProcessOrder is test
	if len(r.Items) != 1 {
		t.Fatalf("want 1, got %d", len(r.Items))
	}
}

func TestDomains(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Domains(ctxBg(), s, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 3 {
		t.Fatalf("want 3 modules, got %d", len(r.Items))
	}
}

func TestDomain(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Domain(ctxBg(), s, "order", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 3 {
		t.Fatalf("want 3 in order, got %d", len(r.Items))
	}
}

func TestCoupling(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Coupling(ctxBg(), s, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) < 2 {
		t.Fatalf("want at least 2 cross-module pairs, got %d", len(r.Items))
	}
}

func TestUntested(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Untested(ctxBg(), s, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// main, validate, saveOrder are untested; ProcessOrder has TestProcessOrder
	if len(r.Items) != 3 {
		names := make([]string, len(r.Items))
		for i, ref := range r.Items {
			names[i] = ref.Name
		}
		t.Fatalf("want 3 untested, got %d: %v", len(r.Items), names)
	}
}

func TestTestCoverage(t *testing.T) {
	s, _ := openFixture(t)
	r, err := TestCoverage(ctxBg(), s, "ProcessOrder", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 1 || r.Items[0].Name != "TestProcessOrder" {
		t.Fatalf("got %+v", r.Items)
	}
}

func TestGraphSummary(t *testing.T) {
	s, _ := openFixture(t)
	sum, err := GraphSummary(ctxBg(), s)
	if err != nil {
		t.Fatal(err)
	}
	if sum.TotalFunctions != 5 {
		t.Errorf("total_functions=%d", sum.TotalFunctions)
	}
	if sum.TotalEdges != 4 {
		t.Errorf("total_edges=%d", sum.TotalEdges)
	}
}

func TestSubgraph(t *testing.T) {
	s, _ := openFixture(t)
	sub, err := Subgraph(ctxBg(), s, "ProcessOrder", 2)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Center == nil {
		t.Fatal("center nil")
	}
}

func TestPagination(t *testing.T) {
	s, _ := openFixture(t)
	r, err := Untested(ctxBg(), s, QueryOpts{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalCount != 3 || r.Returned != 1 || !r.Truncated {
		t.Fatalf("pagination: %+v", r)
	}
}

func TestRefIncludesIdentityFields(t *testing.T) {
	s, _ := openFixture(t)
	r, _ := EntryPoints(ctxBg(), s, UnlimitedOpts())
	if len(r.Items) == 0 {
		t.Fatal("no items")
	}
	ref := r.Items[0]
	if ref.IdentitySource == "" {
		t.Errorf("missing identity_source")
	}
	if ref.Environment == "" {
		t.Errorf("missing environment")
	}
}
