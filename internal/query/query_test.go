package query

import (
	"path/filepath"
	"testing"

	graphdb "github.com/mstrYoda/goraphdb"

	"github.com/codevalent/cvalent/internal/graph"
)

func buildTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "test_graph")
	g, err := graph.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })

	g.CreateSchema()

	// Functions in different modules
	mainFn, _ := g.AddFunction(graphdb.Props{
		"name": "main", "qualified_name": "cmd.main", "file": "cmd/main.go",
		"module": "cmd", "start_line": float64(1), "end_line": float64(5),
		"exported": true, "tag": "application", "contract_completeness": "full",
	})
	processOrder, _ := g.AddFunction(graphdb.Props{
		"name": "ProcessOrder", "qualified_name": "order.ProcessOrder", "file": "order/service.go",
		"module": "order", "start_line": float64(10), "end_line": float64(20),
		"exported": true, "tag": "application", "contract_completeness": "full",
		"contract": `{"parameters":[{"name":"req","type":"OrderRequest"}],"returns":[{"type":"error"}]}`,
	})
	validate, _ := g.AddFunction(graphdb.Props{
		"name": "validate", "qualified_name": "order.validate", "file": "order/validator.go",
		"module": "order", "start_line": float64(5), "end_line": float64(15),
		"exported": false, "tag": "application", "contract_completeness": "full",
	})
	saveOrder, _ := g.AddFunction(graphdb.Props{
		"name": "saveOrder", "qualified_name": "store.saveOrder", "file": "store/repo.go",
		"module": "store", "start_line": float64(10), "end_line": float64(20),
		"exported": false, "tag": "application", "contract_completeness": "full",
	})
	testFn, _ := g.AddTestFunction(graphdb.Props{
		"name": "TestProcessOrder", "qualified_name": "order_test.TestProcessOrder",
		"file": "order/service_test.go", "module": "order",
		"start_line": float64(5), "end_line": float64(15),
		"exported": true, "tag": "test", "contract_completeness": "full",
	})

	// Edges
	g.AddCallEdge(mainFn, processOrder, nil)
	g.AddCallEdge(processOrder, validate, nil)
	g.AddCallEdge(processOrder, saveOrder, nil)
	g.AddCallEdge(testFn, processOrder, nil)

	g.CreateSchema() // rebuild indexes

	return g
}

func TestCallers(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Callers(g, "ProcessOrder", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 { // main and TestProcessOrder
		t.Fatalf("expected 2 callers, got %d", len(result.Items))
	}
}

func TestContract(t *testing.T) {
	g := buildTestGraph(t)
	info, err := Contract(g, "ProcessOrder")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "ProcessOrder" {
		t.Fatalf("expected ProcessOrder, got %s", info.Name)
	}
	if info.Contract == "" {
		t.Fatal("expected non-empty contract")
	}
}

func TestImpact(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Impact(g, "validate", 3, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// validate is called by ProcessOrder, which is called by main and TestProcessOrder
	if len(result.Items) < 1 {
		t.Fatalf("expected at least 1 impacted function, got %d", len(result.Items))
	}
}

func TestEntryPoints(t *testing.T) {
	g := buildTestGraph(t)
	result, err := EntryPoints(g, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// main and TestProcessOrder have no callers
	if len(result.Items) != 2 {
		names := make([]string, len(result.Items))
		for i, ep := range result.Items {
			names[i] = ep.Name
		}
		t.Fatalf("expected 2 entry points, got %d: %v", len(result.Items), names)
	}
}

func TestExports(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Exports(g, "order", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// ProcessOrder is exported, validate is not
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 export, got %d", len(result.Items))
	}
	if result.Items[0].Name != "ProcessOrder" {
		t.Fatalf("expected ProcessOrder, got %s", result.Items[0].Name)
	}
}

func TestDomains(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Domains(g, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 { // cmd, order, store
		t.Fatalf("expected 3 domains, got %d", len(result.Items))
	}
}

func TestDomain(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Domain(g, "order", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 { // ProcessOrder, validate, TestProcessOrder
		t.Fatalf("expected 3 functions in order, got %d", len(result.Items))
	}
}

func TestCoupling(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Coupling(g, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// cmd->order (main->ProcessOrder), order->store (ProcessOrder->saveOrder)
	if len(result.Items) < 2 {
		t.Fatalf("expected at least 2 cross-module edges, got %d", len(result.Items))
	}
}

func TestUntested(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Untested(g, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// main, validate, saveOrder are untested; ProcessOrder has TestProcessOrder
	if len(result.Items) != 3 {
		names := make([]string, len(result.Items))
		for i, u := range result.Items {
			names[i] = u.Name
		}
		t.Fatalf("expected 3 untested, got %d: %v", len(result.Items), names)
	}
}

func TestTestCoverage(t *testing.T) {
	g := buildTestGraph(t)
	result, err := TestCoverage(g, "ProcessOrder", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 test, got %d", len(result.Items))
	}
	if result.Items[0].Name != "TestProcessOrder" {
		t.Fatalf("expected TestProcessOrder, got %s", result.Items[0].Name)
	}
}

func TestBreaks(t *testing.T) {
	g := buildTestGraph(t)
	result, err := Breaks(g, "ProcessOrder", UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	// Same as callers in Phase 1
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 breaks entries, got %d", len(result.Items))
	}
}

func TestPagination(t *testing.T) {
	g := buildTestGraph(t)

	// Untested has 3 results (main, validate, saveOrder)
	// Request with limit=1
	result, err := Untested(g, QueryOpts{Limit: 1, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalCount != 3 {
		t.Fatalf("expected total_count=3, got %d", result.TotalCount)
	}
	if result.Returned != 1 {
		t.Fatalf("expected returned=1, got %d", result.Returned)
	}
	if !result.Truncated {
		t.Fatal("expected truncated=true")
	}

	// Request second page
	result2, err := Untested(g, QueryOpts{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result2.TotalCount != 3 {
		t.Fatalf("expected total_count=3, got %d", result2.TotalCount)
	}
	if result2.Returned != 1 {
		t.Fatalf("expected returned=1, got %d", result2.Returned)
	}
	if result2.Offset != 1 {
		t.Fatalf("expected offset=1, got %d", result2.Offset)
	}
	if !result2.Truncated {
		t.Fatal("expected truncated=true for second page of 3")
	}

	// Request last page
	result3, err := Untested(g, QueryOpts{Limit: 1, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result3.Truncated {
		t.Fatal("expected truncated=false for last page")
	}

	// Request all with no limit
	resultAll, err := Untested(g, UnlimitedOpts())
	if err != nil {
		t.Fatal(err)
	}
	if resultAll.TotalCount != 3 || resultAll.Returned != 3 || resultAll.Truncated {
		t.Fatalf("unlimited should return all: total=%d returned=%d truncated=%v",
			resultAll.TotalCount, resultAll.Returned, resultAll.Truncated)
	}
}
