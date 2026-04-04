package mcp

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
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

	main, _ := g.AddFunction(graphdb.Props{
		"name": "main", "qualified_name": "cmd.main", "file": "cmd/main.go",
		"module": "cmd", "start_line": float64(1), "end_line": float64(5),
		"exported": true, "tag": "application", "contract_completeness": "full",
	})
	process, _ := g.AddFunction(graphdb.Props{
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
	testFn, _ := g.AddTestFunction(graphdb.Props{
		"name": "TestProcessOrder", "qualified_name": "order_test.TestProcessOrder",
		"file": "order/service_test.go", "module": "order",
		"start_line": float64(5), "end_line": float64(15),
		"exported": true, "tag": "test", "contract_completeness": "full",
	})

	g.AddCallEdge(main, process, nil)
	g.AddCallEdge(process, validate, nil)
	g.AddCallEdge(testFn, process, nil)

	g.CreateSchema()
	return g
}

func sendRequest(t *testing.T, s *Server, method string, params interface{}) jsonrpcResponse {
	t.Helper()
	paramsJSON, _ := json.Marshal(params)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  paramsJSON,
	}
	reqJSON, _ := json.Marshal(req)

	input := bytes.NewBufferString(string(reqJSON) + "\n")
	output := &bytes.Buffer{}

	// Run one request through the server
	resp := s.handleRequest(req)
	_ = input
	_ = output
	return resp
}

func TestServerInitialize(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)
	resp := sendRequest(t, s, "initialize", nil)

	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocol version: %v", result["protocolVersion"])
	}
}

func TestToolsList(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)
	resp := sendRequest(t, s, "tools/list", nil)

	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]toolDef)
	if len(tools) != 13 { // 11 queries + graph_summary + subgraph
		t.Fatalf("expected 13 tools, got %d", len(tools))
	}
}

func TestToolCall_Callers(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)

	resp := s.handleToolCall(jsonrpcRequest{
		JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "callers",
			Arguments: mustJSONRaw(t, map[string]string{"function": "ProcessOrder"}),
		}),
	})

	if resp.Error != nil {
		t.Fatal(resp.Error.Message)
	}

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)
	if !strings.Contains(text, "total_count") {
		t.Fatalf("expected envelope with total_count, got: %s", text)
	}
	if !strings.Contains(text, "main") && !strings.Contains(text, "TestProcessOrder") {
		t.Fatalf("expected callers in response, got: %s", text)
	}
}

func TestToolCall_GraphSummary(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)

	resp := s.handleToolCall(jsonrpcRequest{
		JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{Name: "graph_summary"}),
	})

	if resp.Error != nil {
		t.Fatal(resp.Error.Message)
	}

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)
	if !strings.Contains(text, "total_functions") {
		t.Fatalf("expected total_functions in response, got: %s", text)
	}
}

func TestToolCall_Subgraph(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)

	resp := s.handleToolCall(jsonrpcRequest{
		JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "subgraph",
			Arguments: mustJSONRaw(t, map[string]interface{}{"function": "ProcessOrder", "hops": 2}),
		}),
	})

	if resp.Error != nil {
		t.Fatal(resp.Error.Message)
	}

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)
	if !strings.Contains(text, "center") {
		t.Fatalf("expected center in response, got: %s", text)
	}
}

func TestToolCall_UnknownTool(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)

	resp := s.handleToolCall(jsonrpcRequest{
		JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{Name: "nonexistent"}),
	})

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)
	if !strings.Contains(text, "Error") {
		t.Fatalf("expected error for unknown tool, got: %s", text)
	}
}

func TestToolCall_PaginationTruncation(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)

	// entry_points returns 2 items (main, TestProcessOrder). Request limit=1.
	resp := s.handleToolCall(jsonrpcRequest{
		JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "entry_points",
			Arguments: mustJSONRaw(t, map[string]interface{}{"limit": 1}),
		}),
	})

	if resp.Error != nil {
		t.Fatal(resp.Error.Message)
	}

	result := resp.Result.(map[string]interface{})
	content := result["content"].([]map[string]interface{})
	text := content[0]["text"].(string)

	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		t.Fatalf("failed to parse envelope: %v\ntext: %s", err, text)
	}

	if int(envelope["total_count"].(float64)) != 2 {
		t.Fatalf("expected total_count=2, got %v", envelope["total_count"])
	}
	if int(envelope["returned"].(float64)) != 1 {
		t.Fatalf("expected returned=1, got %v", envelope["returned"])
	}
	if envelope["truncated"] != true {
		t.Fatalf("expected truncated=true, got %v", envelope["truncated"])
	}
	if hint, ok := envelope["hint"].(string); !ok || hint == "" {
		t.Fatal("expected non-empty hint when truncated")
	}
}

func TestSessionScaffolded(t *testing.T) {
	g := buildTestGraph(t)
	s := NewServer(g)
	if s.session == nil {
		t.Fatal("expected session to be initialized")
	}
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustJSONRaw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
