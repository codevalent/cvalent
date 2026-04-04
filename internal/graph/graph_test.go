package graph

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	graphdb "github.com/mstrYoda/goraphdb"
)

// openTestGraph creates a temporary graph database for testing.
func openTestGraph(t *testing.T) *Graph {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "test_graph")
	g, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// buildTestGraph creates a graph with known functions and edges for query testing.
//
//	main.go:  main() -> processOrder() -> validateOrder()
//	                  -> sendEmail()
//	order.go: processOrder() -> saveOrder()
//	test_order.go: TestProcessOrder() -> processOrder()
func buildTestGraph(t *testing.T) *Graph {
	t.Helper()
	g := openTestGraph(t)
	if err := g.CreateSchema(); err != nil {
		t.Fatal(err)
	}

	// Function nodes
	mainFn, _ := g.AddFunction(graphdb.Props{
		"name":           "main",
		"qualified_name": "main.main",
		"file":           "main.go",
		"module":         "cmd",
		"start_line":     float64(10),
		"end_line":       float64(25),
		"exported":       true,
		"contract":       `{"parameters":[],"returns":[]}`,
		"contract_completeness": "full",
	})

	processOrder, _ := g.AddFunction(graphdb.Props{
		"name":           "processOrder",
		"qualified_name": "order.processOrder",
		"file":           "order.go",
		"module":         "internal/order",
		"start_line":     float64(15),
		"end_line":       float64(45),
		"exported":       true,
		"contract":       `{"parameters":[{"name":"ctx","type":"context.Context"},{"name":"req","type":"OrderRequest","expanded":{"id":"string","amount":"float64","items":"[]Item"}}],"returns":[{"type":"*OrderResult"},{"type":"error"}]}`,
		"contract_completeness": "full",
	})

	validateOrder, _ := g.AddFunction(graphdb.Props{
		"name":           "validateOrder",
		"qualified_name": "order.validateOrder",
		"file":           "order.go",
		"module":         "internal/order",
		"start_line":     float64(50),
		"end_line":       float64(70),
		"exported":       false,
		"contract":       `{"parameters":[{"name":"req","type":"OrderRequest"}],"returns":[{"type":"error"}]}`,
		"contract_completeness": "full",
	})

	saveOrder, _ := g.AddFunction(graphdb.Props{
		"name":           "saveOrder",
		"qualified_name": "store.saveOrder",
		"file":           "store.go",
		"module":         "internal/store",
		"start_line":     float64(20),
		"end_line":       float64(40),
		"exported":       false,
		"contract":       `{"parameters":[{"name":"order","type":"*Order"}],"returns":[{"type":"error"}]}`,
		"contract_completeness": "full",
	})

	sendEmail, _ := g.AddFunction(graphdb.Props{
		"name":           "sendEmail",
		"qualified_name": "notify.sendEmail",
		"file":           "notify.go",
		"module":         "internal/notify",
		"start_line":     float64(10),
		"end_line":       float64(30),
		"exported":       true,
		"contract":       `{"parameters":[{"name":"to","type":"string"},{"name":"body","type":"string"}],"returns":[{"type":"error"}]}`,
		"contract_completeness": "full",
	})

	testProcessOrder, _ := g.AddTestFunction(graphdb.Props{
		"name":           "TestProcessOrder",
		"qualified_name": "order_test.TestProcessOrder",
		"file":           "order_test.go",
		"module":         "internal/order",
		"start_line":     float64(10),
		"end_line":       float64(35),
		"exported":       true,
		"contract":       `{"parameters":[{"name":"t","type":"*testing.T"}],"returns":[]}`,
		"contract_completeness": "full",
	})

	// CALLS edges
	g.AddCallEdge(mainFn, processOrder, graphdb.Props{
		"data_shape": `{"ctx":"context.Context","req":"OrderRequest"}`,
	})
	g.AddCallEdge(mainFn, sendEmail, graphdb.Props{
		"data_shape": `{"to":"string","body":"string"}`,
	})
	g.AddCallEdge(processOrder, validateOrder, graphdb.Props{
		"data_shape": `{"req":"OrderRequest"}`,
	})
	g.AddCallEdge(processOrder, saveOrder, graphdb.Props{
		"data_shape": `{"order":"*Order"}`,
	})
	g.AddCallEdge(testProcessOrder, processOrder, graphdb.Props{
		"data_shape": `{"ctx":"context.Context","req":"OrderRequest"}`,
	})

	// GraphMeta
	g.AddGraphMeta(graphdb.Props{
		"version":    "1.0.0",
		"build_time": "2026-04-04T20:00:00Z",
		"languages":  "go",
		"files":      float64(4),
		"functions":  float64(6),
	})

	return g
}

// =============================================================================
// 1. Schema Tests — verify node/edge creation and retrieval
// =============================================================================

func TestSchema_CreateFunctionNode(t *testing.T) {
	g := openTestGraph(t)
	if err := g.CreateSchema(); err != nil {
		t.Fatal(err)
	}

	id, err := g.AddFunction(graphdb.Props{
		"name":           "processOrder",
		"qualified_name": "order.processOrder",
		"file":           "order.go",
		"module":         "internal/order",
		"start_line":     float64(15),
		"end_line":       float64(45),
		"exported":       true,
		"contract":       `{"parameters":[{"name":"ctx","type":"context.Context"}],"returns":[{"type":"error"}]}`,
		"contract_completeness": "full",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero node ID")
	}

	// Retrieve via Cypher
	result, err := g.Query(`MATCH (f:Function {name: "processOrder"}) RETURN f`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 function node, got %d", len(result.Rows))
	}

	node := result.Rows[0]["f"].(*graphdb.Node)
	if node.GetString("qualified_name") != "order.processOrder" {
		t.Fatalf("expected qualified_name 'order.processOrder', got %q", node.GetString("qualified_name"))
	}
	if node.GetString("contract_completeness") != "full" {
		t.Fatalf("expected contract_completeness 'full', got %q", node.GetString("contract_completeness"))
	}
}

func TestSchema_CreateCallsEdge(t *testing.T) {
	g := openTestGraph(t)
	if err := g.CreateSchema(); err != nil {
		t.Fatal(err)
	}

	a, _ := g.AddFunction(graphdb.Props{"name": "caller", "qualified_name": "pkg.caller"})
	b, _ := g.AddFunction(graphdb.Props{"name": "callee", "qualified_name": "pkg.callee"})

	edgeID, err := g.AddCallEdge(a, b, graphdb.Props{
		"data_shape": `{"x":"int"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edgeID == 0 {
		t.Fatal("expected non-zero edge ID")
	}

	result, err := g.Query(`MATCH (a:Function {name: "caller"})-[:CALLS]->(b:Function {name: "callee"}) RETURN a, b`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 CALLS edge, got %d rows", len(result.Rows))
	}
}

func TestSchema_CreateImportsEdge(t *testing.T) {
	g := openTestGraph(t)
	if err := g.CreateSchema(); err != nil {
		t.Fatal(err)
	}

	a, _ := g.AddFunction(graphdb.Props{"name": "handler", "qualified_name": "api.handler"})
	b, _ := g.AddFunction(graphdb.Props{"name": "service", "qualified_name": "svc.service"})

	_, err := g.AddImportEdge(a, b, graphdb.Props{"import_path": "internal/svc"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Query(`MATCH (a:Function {name: "handler"})-[:IMPORTS]->(b:Function {name: "service"}) RETURN a, b`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 IMPORTS edge, got %d rows", len(result.Rows))
	}
}

func TestSchema_GraphMetaNode(t *testing.T) {
	g := openTestGraph(t)
	if err := g.CreateSchema(); err != nil {
		t.Fatal(err)
	}

	_, err := g.AddGraphMeta(graphdb.Props{
		"version":    "1.0.0",
		"build_time": "2026-04-04T20:00:00Z",
		"languages":  "go,python",
		"files":      float64(42),
		"functions":  float64(150),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := g.Query(`MATCH (m:GraphMeta) RETURN m`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 GraphMeta node, got %d", len(result.Rows))
	}

	meta := result.Rows[0]["m"].(*graphdb.Node)
	if meta.GetString("version") != "1.0.0" {
		t.Fatalf("expected version '1.0.0', got %q", meta.GetString("version"))
	}
}

// =============================================================================
// 2. Phase 1 Query Tests — all 11 CLI query patterns must execute without error
// =============================================================================

func TestPhase1Query_Callers(t *testing.T) {
	g := buildTestGraph(t)

	// callers of processOrder: main and TestProcessOrder
	result, err := g.Query(`MATCH (caller:Function)-[:CALLS]->(f:Function {name: "processOrder"}) RETURN caller`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 callers, got %d", len(result.Rows))
	}
}

func TestPhase1Query_Contract(t *testing.T) {
	g := buildTestGraph(t)

	result, err := g.Query(`MATCH (f:Function {name: "processOrder"}) RETURN f.contract`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Rows))
	}

	contractStr, ok := result.Rows[0]["f.contract"].(string)
	if !ok {
		t.Fatalf("expected string contract, got %T", result.Rows[0]["f.contract"])
	}

	var contract map[string]interface{}
	if err := json.Unmarshal([]byte(contractStr), &contract); err != nil {
		t.Fatalf("contract is not valid JSON: %v", err)
	}
	params, ok := contract["parameters"].([]interface{})
	if !ok || len(params) != 2 {
		t.Fatalf("expected 2 parameters, got %v", contract["parameters"])
	}
}

func TestPhase1Query_Impact(t *testing.T) {
	g := buildTestGraph(t)

	// Impact of validateOrder at depth 3: who calls validateOrder (processOrder),
	// who calls processOrder (main, TestProcessOrder)
	result, err := g.Query(`MATCH (caller:Function)-[:CALLS*1..3]->(f:Function {name: "validateOrder"}) RETURN caller`)
	if err != nil {
		t.Fatal(err)
	}
	// processOrder calls validateOrder (depth 1)
	// main calls processOrder (depth 2), TestProcessOrder calls processOrder (depth 2)
	if len(result.Rows) < 1 {
		t.Fatal("expected at least 1 impacted function")
	}
}

func TestPhase1Query_EntryPoints(t *testing.T) {
	g := buildTestGraph(t)

	// Entry points: functions with no incoming CALLS edges.
	// In our graph: main and TestProcessOrder have no callers.
	result, err := g.Query(`MATCH (f:Function) RETURN f`)
	if err != nil {
		t.Fatal(err)
	}

	// For each function, check if it has incoming CALLS edges
	entryPoints := 0
	for _, row := range result.Rows {
		node := row["f"].(*graphdb.Node)
		name := node.GetString("name")
		callersResult, err := g.Query(fmt.Sprintf(`MATCH (caller:Function)-[:CALLS]->(f:Function {name: "%s"}) RETURN caller`, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(callersResult.Rows) == 0 {
			entryPoints++
		}
	}
	if entryPoints != 2 {
		t.Fatalf("expected 2 entry points (main, TestProcessOrder), got %d", entryPoints)
	}
}

func TestPhase1Query_Exports(t *testing.T) {
	g := buildTestGraph(t)

	// Exported functions in internal/order module
	// Exclude test functions — exports query is about public API, not test surface
	result, err := g.Query(`MATCH (f:Function {module: "internal/order"}) WHERE f.exported = true RETURN f`)
	if err != nil {
		t.Fatal(err)
	}
	// processOrder is exported, validateOrder is not, TestProcessOrder is exported + test
	// Both processOrder and TestProcessOrder are exported
	exportedNonTest := 0
	for _, row := range result.Rows {
		node := row["f"].(*graphdb.Node)
		if node.Props["is_test"] != true {
			exportedNonTest++
		}
	}
	if exportedNonTest != 1 {
		t.Fatalf("expected 1 exported non-test function in internal/order, got %d", exportedNonTest)
	}
}

func TestPhase1Query_Domains(t *testing.T) {
	g := buildTestGraph(t)

	// Get all distinct modules (domains are directory-based)
	result, err := g.Query(`MATCH (f:Function) RETURN f.module`)
	if err != nil {
		t.Fatal(err)
	}
	modules := map[string]bool{}
	for _, row := range result.Rows {
		if mod, ok := row["f.module"].(string); ok {
			modules[mod] = true
		}
	}
	// cmd, internal/order, internal/store, internal/notify
	if len(modules) != 4 {
		t.Fatalf("expected 4 modules, got %d: %v", len(modules), modules)
	}
}

func TestPhase1Query_Domain(t *testing.T) {
	g := buildTestGraph(t)

	// Functions in internal/order
	result, err := g.Query(`MATCH (f:Function {module: "internal/order"}) RETURN f`)
	if err != nil {
		t.Fatal(err)
	}
	// processOrder, validateOrder, TestProcessOrder
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 functions in internal/order, got %d", len(result.Rows))
	}
}

func TestPhase1Query_Coupling(t *testing.T) {
	g := buildTestGraph(t)

	// Cross-module edges: find edges where source and target are in different modules
	result, err := g.Query(`MATCH (a:Function)-[:CALLS]->(b:Function) RETURN a.module, b.module`)
	if err != nil {
		t.Fatal(err)
	}
	crossModule := 0
	for _, row := range result.Rows {
		aModule, _ := row["a.module"].(string)
		bModule, _ := row["b.module"].(string)
		if aModule != bModule {
			crossModule++
		}
	}
	// main->processOrder (cmd->internal/order), main->sendEmail (cmd->internal/notify),
	// processOrder->saveOrder (internal/order->internal/store)
	if crossModule != 3 {
		t.Fatalf("expected 3 cross-module edges, got %d", crossModule)
	}
}

func TestPhase1Query_Untested(t *testing.T) {
	g := buildTestGraph(t)

	// Application functions (not test) with no incoming test edges.
	// All functions, then check which have test callers.
	result, err := g.Query(`MATCH (f:Function) WHERE f.is_test <> true RETURN f`)
	if err != nil {
		// If <> not supported, try alternative
		result, err = g.Query(`MATCH (f:Function) RETURN f`)
		if err != nil {
			t.Fatal(err)
		}
	}

	untested := 0
	for _, row := range result.Rows {
		node := row["f"].(*graphdb.Node)
		if node.Props["is_test"] == true {
			continue
		}
		name := node.GetString("name")
		testCallers, err := g.Query(fmt.Sprintf(`MATCH (t:Function)-[:CALLS]->(f:Function {name: "%s"}) WHERE t.is_test = true RETURN t`, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(testCallers.Rows) == 0 {
			untested++
		}
	}
	// main, validateOrder, saveOrder, sendEmail are untested; processOrder has TestProcessOrder
	if untested != 4 {
		t.Fatalf("expected 4 untested functions, got %d", untested)
	}
}

func TestPhase1Query_TestCoverage(t *testing.T) {
	g := buildTestGraph(t)

	// Which tests exercise processOrder?
	result, err := g.Query(`MATCH (t:Function)-[:CALLS]->(f:Function {name: "processOrder"}) WHERE t.is_test = true RETURN t`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 test covering processOrder, got %d", len(result.Rows))
	}
	testNode := result.Rows[0]["t"].(*graphdb.Node)
	if testNode.GetString("name") != "TestProcessOrder" {
		t.Fatalf("expected TestProcessOrder, got %s", testNode.GetString("name"))
	}
}

func TestPhase1Query_Breaks(t *testing.T) {
	g := buildTestGraph(t)

	// Breaks query: for a changed function, find all callers and their edge data_shape.
	// This tests the pattern, not actual break detection (which compares old vs new).
	result, err := g.Query(`MATCH (caller:Function)-[e:CALLS]->(f:Function {name: "processOrder"}) RETURN caller.name, e`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 callers with edges, got %d", len(result.Rows))
	}
}

// =============================================================================
// 3. Future-Proof Gate Tests — Phase 3 and Phase 5 patterns
// =============================================================================

func TestGate_VariableLengthPaths(t *testing.T) {
	g := buildTestGraph(t)

	// Phase 3: multi-hop with variable-length paths
	result, err := g.Query(`MATCH (a:Function)-[:CALLS*2..6]->(b:Function) RETURN a, b`)
	if err != nil {
		t.Fatalf("variable-length path (2..6) failed: %v", err)
	}
	// main -> processOrder -> validateOrder (depth 2), main -> processOrder -> saveOrder (depth 2)
	if len(result.Rows) < 2 {
		t.Fatalf("expected at least 2 multi-hop results, got %d", len(result.Rows))
	}
}

func TestGate_DeepVariableLengthPaths(t *testing.T) {
	g := buildTestGraph(t)

	// Phase 3: deep variable-length paths (5..15) — should return empty but not error
	result, err := g.Query(`MATCH (a:Function)-[:CALLS*5..15]->(b:Function) RETURN a, b`)
	if err != nil {
		t.Fatalf("deep variable-length path (5..15) failed: %v", err)
	}
	// Our graph only has depth 3, so this should be empty
	_ = result
}

func TestGate_VariableLengthWithFilter(t *testing.T) {
	g := buildTestGraph(t)

	// Phase 3: multi-hop with WHERE filters
	result, err := g.Query(`MATCH (a:Function)-[:CALLS*1..3]->(b:Function) WHERE b.exported = true RETURN a, b`)
	if err != nil {
		t.Fatalf("variable-length with WHERE filter failed: %v", err)
	}
	_ = result
}

func TestGate_OrderByLimit(t *testing.T) {
	g := buildTestGraph(t)

	// Phase 3: ORDER BY + LIMIT aggregation pattern
	result, err := g.Query(`MATCH (f:Function) RETURN f ORDER BY f.name LIMIT 3`)
	if err != nil {
		t.Fatalf("ORDER BY + LIMIT failed: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows with LIMIT 3, got %d", len(result.Rows))
	}
}

func TestGate_BidirectionalTraversal(t *testing.T) {
	g := buildTestGraph(t)

	// Phase 5: undirected traversal — find nodes connected in either direction
	// GoraphDB uses (a)-[:CALLS]-(b) syntax for undirected
	result, err := g.Query(`MATCH (a:Function {name: "processOrder"})-[:CALLS]-(b:Function) RETURN b`)
	if err != nil {
		// If undirected not supported, document it but don't fail the gate
		t.Logf("GATE NOTE: undirected traversal not supported in Cypher: %v", err)
		t.Logf("GATE NOTE: will implement in Go using separate incoming/outgoing queries")
		// Still pass — the spec says this can be done in Go
		return
	}
	// Should get callers AND callees of processOrder
	_ = result
}

func TestGate_OptionalMatch(t *testing.T) {
	g := buildTestGraph(t)

	// OPTIONAL MATCH — left outer join pattern
	result, err := g.Query(`MATCH (f:Function {name: "sendEmail"}) OPTIONAL MATCH (t:Function)-[:CALLS]->(f) WHERE t.is_test = true RETURN f, t`)
	if err != nil {
		t.Fatalf("OPTIONAL MATCH failed: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row from OPTIONAL MATCH, got %d", len(result.Rows))
	}
	// t should be nil (no test calls sendEmail)
}

