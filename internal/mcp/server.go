// Package mcp implements the cvalent MCP (Model Context Protocol)
// server over stdio. The server hosts the 13 tools defined by Q9.
//
// Friction wrapping (the `boundaries` envelope on seven of the tools)
// lives in internal/friction and lands as AH-0316.16/.17.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/codevalent/cvalent/internal/friction"
	"github.com/codevalent/cvalent/internal/query"
	"github.com/codevalent/cvalent/internal/store"
)

// Server implements MCP over stdio.
type Server struct {
	store    *store.Store
	friction friction.Detector
	session  *Session
}

// Session scaffolds context tracking for future hosted-store sessions.
type Session struct {
	QueriedFunctions []string `json:"queried_functions"`
	ActiveModule     string   `json:"active_module"`
}

// NewServer creates an MCP server backed by the given store.
func NewServer(s *store.Store) *Server {
	return &Server{store: s, friction: friction.New(), session: &Session{}}
}

// JSON-RPC types
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string             `json:"type"`
	Properties map[string]propDef `json:"properties,omitempty"`
	Required   []string           `json:"required,omitempty"`
}

type propDef struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Default     *float64 `json:"default,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
}

func numPtr(v float64) *float64 { return &v }

var paginationProps = map[string]propDef{
	"limit":  {Type: "number", Description: "Max results to return (default 50, 0 for unlimited)", Default: numPtr(50), Minimum: numPtr(0)},
	"offset": {Type: "number", Description: "Number of results to skip (default 0)", Default: numPtr(0), Minimum: numPtr(0)},
}

func withPagination(props map[string]propDef) map[string]propDef {
	merged := make(map[string]propDef, len(props)+2)
	for k, v := range props {
		merged[k] = v
	}
	for k, v := range paginationProps {
		merged[k] = v
	}
	return merged
}

func (s *Server) tools() []toolDef {
	fnProp := map[string]propDef{"function": {Type: "string", Description: "Fully qualified or short function name to look up"}}
	modProp := map[string]propDef{"module": {Type: "string", Description: "Module name (directory-based grouping)"}}

	return []toolDef{
		{Name: "callers", Description: "List all functions that directly call the specified function.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(fnProp), Required: []string{"function"}}},
		{Name: "contract", Description: "Return the parameter and return-type contract of a function.",
			InputSchema: inputSchema{Type: "object", Properties: fnProp, Required: []string{"function"}}},
		{Name: "impact", Description: "Trace downstream callers to N levels, showing the blast radius of a change.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(map[string]propDef{
				"function": {Type: "string", Description: "Fully qualified or short function name to look up"},
				"depth":    {Type: "number", Description: "Maximum traversal depth (default: 3)", Default: numPtr(3), Minimum: numPtr(1), Maximum: numPtr(10)},
			}), Required: []string{"function"}}},
		{Name: "breaks", Description: "Detect callers whose argument shape mismatches the function contract.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(fnProp), Required: []string{"function"}}},
		{Name: "entry_points", Description: "List all functions with no incoming call edges.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(nil)}},
		{Name: "exports", Description: "List the public (exported) functions of a module.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(modProp), Required: []string{"module"}}},
		{Name: "domains", Description: "List all directory-based module groupings with function counts.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(nil)}},
		{Name: "domain", Description: "List functions and internal call edges within a single module.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(modProp), Required: []string{"module"}}},
		{Name: "coupling", Description: "Measure cross-module dependency density across all module pairs.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(nil)}},
		{Name: "untested", Description: "List application functions that have no test coverage.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(nil)}},
		{Name: "test_coverage", Description: "List the test functions that exercise the specified function.",
			InputSchema: inputSchema{Type: "object", Properties: withPagination(fnProp), Required: []string{"function"}}},
		{Name: "graph_summary", Description: "Return a compact overview of the pre-built code graph: modules, counts, coverage.",
			InputSchema: inputSchema{Type: "object"}},
		{Name: "subgraph", Description: "Extract the N-hop neighborhood of a function with contracts and edges.",
			InputSchema: inputSchema{Type: "object", Properties: map[string]propDef{
				"function": {Type: "string", Description: "Fully qualified or short function name to look up"},
				"hops":     {Type: "number", Description: "Neighborhood radius in call-graph hops (default: 2)", Default: numPtr(2), Minimum: numPtr(1), Maximum: numPtr(5)},
			}, Required: []string{"function"}}},
	}
}

// Serve runs the MCP server reading from r and writing to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var req jsonrpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		resp := s.handleRequest(req)
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "%s\n", data)
	}
	return scanner.Err()
}

func (s *Server) handleRequest(req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo": map[string]interface{}{
					"name":    "cvalent",
					"version": "0.2.0-dev",
				},
			},
		}
	case "tools/list":
		return jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]interface{}{"tools": s.tools()},
		}
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(req jsonrpcRequest) jsonrpcResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &rpcError{Code: -32602, Message: "invalid params"}}
	}
	var args map[string]interface{}
	if params.Arguments != nil {
		json.Unmarshal(params.Arguments, &args)
	}
	result, err := s.callTool(params.Name, args)
	if err != nil {
		return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "Error: " + err.Error()},
				},
			},
		}
	}
	text, _ := json.Marshal(result)
	return jsonrpcResponse{JSONRPC: "2.0", ID: req.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": string(text)},
			},
		},
	}
}

const defaultMCPLimit = 50

func mcpOpts(args map[string]interface{}) query.QueryOpts {
	opts := query.QueryOpts{Limit: defaultMCPLimit}
	if v, ok := args["limit"]; ok {
		if n, ok := v.(float64); ok {
			opts.Limit = int(n)
		}
	}
	if v, ok := args["offset"]; ok {
		if n, ok := v.(float64); ok {
			opts.Offset = int(n)
		}
	}
	return opts
}

type pagedEnvelope struct {
	TotalCount     int                  `json:"total_count"`
	Returned       int                  `json:"returned"`
	Offset         int                  `json:"offset"`
	Items          interface{}          `json:"items"`
	Truncated      bool                 `json:"truncated"`
	Hint           string               `json:"hint,omitempty"`
	Boundaries     *[]friction.Boundary `json:"boundaries,omitempty"`
	BoundarySignal string               `json:"boundary_signal,omitempty"`
}

func wrapPaged[T any](pr query.PagedResult[T]) pagedEnvelope {
	items := pr.Items
	if items == nil {
		items = make([]T, 0)
	}
	env := pagedEnvelope{
		TotalCount: pr.TotalCount,
		Returned:   pr.Returned,
		Offset:     pr.Offset,
		Items:      items,
		Truncated:  pr.Truncated,
	}
	if pr.Truncated {
		env.Hint = fmt.Sprintf("Use offset=%d to see more", pr.Offset+pr.Returned)
	}
	return env
}

// attachBoundaries wraps an envelope with boundaries from the friction
// detector. For the seven affected tools, the field is always present
// (possibly empty array). For the six unaffected tools, the caller
// must not invoke this — the JSON `omitempty` ensures the field is
// dropped when nil.
func (s *Server) attachBoundaries(env *pagedEnvelope, tool string, args map[string]any, result any) {
	if !friction.HasBoundary(tool) {
		return
	}
	bs := s.friction.Detect(context.Background(), s.store, tool, args, result)
	if bs == nil {
		bs = []friction.Boundary{}
	}
	env.Boundaries = &bs
	env.BoundarySignal = friction.BoundarySignal
}

func (s *Server) callTool(name string, args map[string]interface{}) (interface{}, error) {
	ctx := context.Background()
	getStr := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	getInt := func(key string, def int) int {
		if v, ok := args[key]; ok {
			if n, ok := v.(float64); ok {
				return int(n)
			}
		}
		return def
	}
	opts := mcpOpts(args)

	wrapAffected := func(name string, r any) any {
		switch v := r.(type) {
		case query.PagedResult[query.FunctionRef]:
			env := wrapPaged(v)
			s.attachBoundaries(&env, name, args, v)
			return env
		}
		return r
	}

	switch name {
	case "callers":
		r, err := query.Callers(ctx, s.store, getStr("function"), opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "contract":
		return query.Contract(ctx, s.store, getStr("function"))
	case "impact":
		r, err := query.Impact(ctx, s.store, getStr("function"), getInt("depth", 3), opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "breaks":
		r, err := query.Breaks(ctx, s.store, getStr("function"), opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "entry_points":
		r, err := query.EntryPoints(ctx, s.store, opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "exports":
		r, err := query.Exports(ctx, s.store, getStr("module"), opts)
		if err != nil {
			return nil, err
		}
		return wrapPaged(r), nil
	case "domains":
		r, err := query.Domains(ctx, s.store, opts)
		if err != nil {
			return nil, err
		}
		return wrapPaged(r), nil
	case "domain":
		r, err := query.Domain(ctx, s.store, getStr("module"), opts)
		if err != nil {
			return nil, err
		}
		return wrapPaged(r), nil
	case "coupling":
		r, err := query.Coupling(ctx, s.store, opts)
		if err != nil {
			return nil, err
		}
		return wrapPaged(r), nil
	case "untested":
		r, err := query.Untested(ctx, s.store, opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "test_coverage":
		r, err := query.TestCoverage(ctx, s.store, getStr("function"), opts)
		if err != nil {
			return nil, err
		}
		return wrapAffected(name, r), nil
	case "graph_summary":
		return query.GraphSummary(ctx, s.store)
	case "subgraph":
		sub, err := query.Subgraph(ctx, s.store, getStr("function"), getInt("hops", 2))
		if err != nil {
			return nil, err
		}
		// Subgraph response carries its own boundaries field on a
		// dedicated envelope so the shape is consistent with the other
		// six affected tools.
		bs := s.friction.Detect(ctx, s.store, name, args, sub)
		if bs == nil {
			bs = []friction.Boundary{}
		}
		return subgraphEnvelope{
			Center:         sub.Center,
			Callers:        sub.Callers,
			Callees:        sub.Callees,
			Hops:           sub.Hops,
			Boundaries:     bs,
			BoundarySignal: friction.BoundarySignal,
		}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// subgraphEnvelope is the wire shape for the `subgraph` MCP tool —
// the center plus its neighbours, plus the friction envelope (subgraph
// is one of the seven affected tools).
type subgraphEnvelope struct {
	Center         *query.FunctionDetail `json:"center"`
	Callers        []query.FunctionRef   `json:"callers"`
	Callees        []query.FunctionRef   `json:"callees"`
	Hops           int                   `json:"hops"`
	Boundaries     []friction.Boundary   `json:"boundaries"`
	BoundarySignal string                `json:"boundary_signal"`
}
