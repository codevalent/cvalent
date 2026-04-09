package mcp

import (
	"bytes"
	"context"
	"encoding/json"
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
		},
	}
}

func buildTestStore(t *testing.T) *store.Store {
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
	test := mintFn(t, "example.com/x", "order", "TestProcessOrder", true)

	for _, fn := range []model.FunctionNode{main, process, validate, test} {
		if err := s.UpsertNode(ctx, fn); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []store.Edge{
		{ID: edgeUUID(main.ID, process.ID), From: main.ID, To: process.ID, Kind: "call"},
		{ID: edgeUUID(process.ID, validate.ID), From: process.ID, To: validate.ID, Kind: "call"},
		{ID: edgeUUID(test.ID, process.ID), From: test.ID, To: process.ID, Kind: "call"},
	} {
		if err := s.UpsertEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func edgeUUID(a, b [16]byte) [16]byte {
	var out [16]byte
	for i := 0; i < 16; i++ {
		out[i] = a[i] ^ b[i]
	}
	out[6] = (out[6] & 0x0f) | 0x50
	out[8] = (out[8] & 0x3f) | 0x80
	return out
}

func sendRequest(t *testing.T, s *Server, method string, params interface{}) jsonrpcResponse {
	t.Helper()
	paramsJSON, _ := json.Marshal(params)
	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: paramsJSON}
	return s.handleRequest(req)
}

func TestServerInitialize(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := sendRequest(t, s, "initialize", nil)
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocol: %v", result["protocolVersion"])
	}
}

func TestToolsList_Has13Tools(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := sendRequest(t, s, "tools/list", nil)
	result := resp.Result.(map[string]interface{})
	tools := result["tools"].([]toolDef)
	if len(tools) != 13 {
		t.Fatalf("expected 13 tools, got %d", len(tools))
	}
}

func TestToolCall_Callers(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "callers",
			Arguments: mustJSONRaw(t, map[string]string{"function": "ProcessOrder"}),
		})})
	if resp.Error != nil {
		t.Fatal(resp.Error.Message)
	}
	text := extractText(t, resp)
	if !strings.Contains(text, "total_count") {
		t.Fatalf("envelope missing: %s", text)
	}
	if !strings.Contains(text, "main") || !strings.Contains(text, "TestProcessOrder") {
		t.Fatalf("expected callers in response: %s", text)
	}
}

func TestToolCall_GraphSummary(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{Name: "graph_summary"})})
	text := extractText(t, resp)
	if !strings.Contains(text, "total_functions") {
		t.Fatalf("missing total_functions: %s", text)
	}
}

func TestToolCall_Subgraph(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "subgraph",
			Arguments: mustJSONRaw(t, map[string]any{"function": "ProcessOrder", "hops": 2}),
		})})
	text := extractText(t, resp)
	if !strings.Contains(text, "center") {
		t.Fatalf("missing center: %s", text)
	}
}

func TestToolCall_UnknownTool(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{Name: "nonexistent"})})
	text := extractText(t, resp)
	if !strings.Contains(text, "Error") {
		t.Fatalf("expected error: %s", text)
	}
}

func TestToolCall_PaginationTruncation(t *testing.T) {
	s := NewServer(buildTestStore(t))
	// entry_points returns at least 2 items; request limit=1.
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "entry_points",
			Arguments: mustJSONRaw(t, map[string]any{"limit": 1}),
		})})
	text := extractText(t, resp)
	var env map[string]interface{}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("envelope parse: %v\n%s", err, text)
	}
	if int(env["returned"].(float64)) != 1 {
		t.Fatalf("returned=%v", env["returned"])
	}
	if env["truncated"] != true {
		t.Fatalf("not truncated: %v", env["truncated"])
	}
}

func TestToolCall_AffectedToolsHaveBoundaries(t *testing.T) {
	s := NewServer(buildTestStore(t))
	for _, tool := range []string{"callers", "impact", "breaks", "test_coverage", "untested", "entry_points"} {
		args := map[string]string{"function": "ProcessOrder"}
		var raw json.RawMessage
		if tool == "untested" || tool == "entry_points" {
			raw = mustJSONRaw(t, map[string]any{})
		} else {
			raw = mustJSONRaw(t, args)
		}
		resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
			Params: mustJSON(t, toolCallParams{Name: tool, Arguments: raw})})
		text := extractText(t, resp)
		var env map[string]interface{}
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			t.Fatalf("%s: parse: %v\n%s", tool, err, text)
		}
		if _, has := env["boundaries"]; !has {
			t.Errorf("%s: boundaries field missing", tool)
		}
		if env["boundary_signal"] != "hosted_resolves_cross_repo" {
			t.Errorf("%s: boundary_signal = %v", tool, env["boundary_signal"])
		}
	}
}

func TestToolCall_UnaffectedToolsOmitBoundaries(t *testing.T) {
	s := NewServer(buildTestStore(t))
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"contract", map[string]any{"function": "ProcessOrder"}},
		{"exports", map[string]any{"module": "order"}},
		{"domains", map[string]any{}},
		{"domain", map[string]any{"module": "order"}},
		{"coupling", map[string]any{}},
		{"graph_summary", map[string]any{}},
	}
	for _, c := range cases {
		resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
			Params: mustJSON(t, toolCallParams{Name: c.tool, Arguments: mustJSONRaw(t, c.args)})})
		text := extractText(t, resp)
		var env map[string]interface{}
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			continue
		}
		if _, has := env["boundaries"]; has {
			t.Errorf("%s: must omit boundaries", c.tool)
		}
		if _, has := env["boundary_signal"]; has {
			t.Errorf("%s: must omit boundary_signal", c.tool)
		}
	}
}

func TestToolCall_ContractIsFunctionDetail(t *testing.T) {
	s := NewServer(buildTestStore(t))
	resp := s.handleToolCall(jsonrpcRequest{JSONRPC: "2.0", ID: 1,
		Params: mustJSON(t, toolCallParams{
			Name:      "contract",
			Arguments: mustJSONRaw(t, map[string]any{"function": "ProcessOrder"}),
		})})
	text := extractText(t, resp)
	for _, key := range []string{"pipeline_references", "recent_traces", "contract_history", "upstream_storage", "downstream_storage"} {
		if !strings.Contains(text, key) {
			t.Errorf("contract response missing %q\n%s", key, text)
		}
	}
}

func TestSessionScaffolded(t *testing.T) {
	s := NewServer(buildTestStore(t))
	if s.session == nil {
		t.Fatal("session nil")
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
	return mustJSON(t, v)
}

func extractText(t *testing.T, resp jsonrpcResponse) string {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("rpc error: %s", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result shape")
	}
	content := result["content"].([]map[string]interface{})
	text, _ := content[0]["text"].(string)
	if text == "" {
		buf, _ := json.Marshal(resp.Result)
		return string(buf)
	}
	_ = bytes.NewBuffer
	return text
}
