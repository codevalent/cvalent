package graph

import (
	"context"
	"fmt"

	graphdb "github.com/mstrYoda/goraphdb"
)

// Graph wraps GoraphDB and provides the cvalent-specific schema and query interface.
type Graph struct {
	db   *graphdb.DB
	path string
}

// CypherResult wraps query results from GoraphDB.
type CypherResult struct {
	Columns []string
	Rows    []map[string]interface{}
}

// Open creates or opens a graph database at the given path.
func Open(path string) (*Graph, error) {
	opts := graphdb.DefaultOptions()
	opts.NoSync = true
	db, err := graphdb.Open(path, opts)
	if err != nil {
		return nil, err
	}
	return &Graph{db: db, path: path}, nil
}

// Close closes the graph database.
func (g *Graph) Close() error {
	return g.db.Close()
}

// DB returns the underlying GoraphDB instance for direct access.
func (g *Graph) DB() *graphdb.DB {
	return g.db
}

// CreateSchema sets up the cvalent graph schema — indexes for fast property lookups.
// Safe to call multiple times; rebuilds indexes from scratch each time (useful after batch loads).
func (g *Graph) CreateSchema() error {
	indexProps := []string{"name", "qualified_name", "file", "module"}
	for _, prop := range indexProps {
		if err := g.db.ReIndex(prop); err != nil {
			return err
		}
	}
	return nil
}

// AddFunction adds a Function node to the graph with the given properties.
func (g *Graph) AddFunction(props graphdb.Props) (graphdb.NodeID, error) {
	return g.db.AddNodeWithLabels([]string{"Function"}, props)
}

// AddTestFunction adds a Function node tagged as a test function.
func (g *Graph) AddTestFunction(props graphdb.Props) (graphdb.NodeID, error) {
	props["is_test"] = true
	return g.db.AddNodeWithLabels([]string{"Function"}, props)
}

// AddGraphMeta adds or updates the GraphMeta singleton node.
func (g *Graph) AddGraphMeta(props graphdb.Props) (graphdb.NodeID, error) {
	return g.db.AddNodeWithLabels([]string{"GraphMeta"}, props)
}

// AddCallEdge adds a CALLS edge between two functions.
func (g *Graph) AddCallEdge(from, to graphdb.NodeID, props graphdb.Props) (graphdb.EdgeID, error) {
	return g.db.AddEdge(from, to, "CALLS", props)
}

// AddImportEdge adds an IMPORTS edge between two functions/modules.
func (g *Graph) AddImportEdge(from, to graphdb.NodeID, props graphdb.Props) (graphdb.EdgeID, error) {
	return g.db.AddEdge(from, to, "IMPORTS", props)
}

// Query executes a Cypher query and returns the results.
func (g *Graph) Query(cypher string) (*CypherResult, error) {
	ctx := context.Background()
	result, err := g.db.Cypher(ctx, cypher)
	if err != nil {
		return nil, err
	}
	return &CypherResult{
		Columns: result.Columns,
		Rows:    result.Rows,
	}, nil
}

// QueryWithParams executes a parameterized Cypher query.
func (g *Graph) QueryWithParams(cypher string, params map[string]interface{}) (*CypherResult, error) {
	ctx := context.Background()
	result, err := g.db.CypherWithParams(ctx, cypher, params)
	if err != nil {
		return nil, err
	}
	return &CypherResult{
		Columns: result.Columns,
		Rows:    result.Rows,
	}, nil
}

// BulkAddFunctions adds multiple function nodes in a batch using AddNodeBatch.
// Note: after calling this, indexes must be rebuilt with CreateSchema() or ReIndex().
func (g *Graph) BulkAddFunctions(propsList []graphdb.Props) ([]graphdb.NodeID, error) {
	// AddNodeBatch skips labels, so we embed label info in props and use batch insert.
	// We also add a _label prop for filtering, plus use AddNodeBatch for speed.
	for i := range propsList {
		propsList[i]["_label"] = "Function"
	}
	return g.db.AddNodeBatch(propsList)
}

// BulkAddCallEdges adds multiple CALLS edges in a single transaction.
func (g *Graph) BulkAddCallEdges(from, to []graphdb.NodeID) ([]graphdb.EdgeID, error) {
	edges := make([]graphdb.Edge, len(from))
	for i := range from {
		edges[i] = graphdb.Edge{
			From:  from[i],
			To:    to[i],
			Label: "CALLS",
		}
	}
	return g.db.AddEdgeBatch(edges)
}

// FindByName finds a node by its "name" property using the index.
func (g *Graph) FindByName(name string) (*graphdb.Node, error) {
	result, err := g.Query(`MATCH (f {name: "` + name + `"}) RETURN f LIMIT 1`)
	if err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 {
		return nil, nil
	}
	node, ok := result.Rows[0]["f"].(*graphdb.Node)
	if !ok {
		return nil, fmt.Errorf("unexpected type in result: %T", result.Rows[0]["f"])
	}
	return node, nil
}

// Callers returns nodes that have CALLS edges pointing to the given node.
// Uses native InEdges API for O(degree) performance at scale.
func (g *Graph) Callers(nodeID graphdb.NodeID) ([]*graphdb.Node, error) {
	inEdges, err := g.db.InEdgesLabeled(nodeID, "CALLS")
	if err != nil {
		return nil, err
	}
	nodes := make([]*graphdb.Node, 0, len(inEdges))
	for _, edge := range inEdges {
		node, err := g.db.GetNode(edge.From)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// NodeCount returns the total number of nodes in the graph.
func (g *Graph) NodeCount() uint64 {
	return g.db.NodeCount()
}

// EdgeCount returns the total number of edges in the graph.
func (g *Graph) EdgeCount() uint64 {
	return g.db.EdgeCount()
}
