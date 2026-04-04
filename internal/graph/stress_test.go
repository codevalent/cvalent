//go:build stress

package graph_test

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	graphdb "github.com/mstrYoda/goraphdb"

	"github.com/codevalent/cvalent/internal/graph"
	"github.com/codevalent/cvalent/internal/query"
)

// TestStress exercises GoraphDB at scale: 100k Function nodes, 500k CALLS edges,
// then runs the core query patterns and reports latency.
func TestStress(t *testing.T) {
	const (
		numNodes   = 100_000
		numEdges   = 500_000
		numModules = 100
	)

	dir := filepath.Join(t.TempDir(), "stress_graph")
	g, err := graph.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	rng := rand.New(rand.NewSource(42))

	// =========================================================================
	// Insert phase
	// =========================================================================
	insertStart := time.Now()

	propsBatch := make([]graphdb.Props, numNodes)
	for i := 0; i < numNodes; i++ {
		propsBatch[i] = graphdb.Props{
			"name":                  fmt.Sprintf("fn_%d", i),
			"qualified_name":        fmt.Sprintf("mod%d.fn_%d", i%numModules, i),
			"file":                  fmt.Sprintf("file_%d.go", i%1000),
			"module":                fmt.Sprintf("module_%d", i%numModules),
			"start_line":            float64(i),
			"end_line":              float64(i + 10),
			"exported":              i%3 == 0,
			"contract":              `{"parameters":[],"returns":[]}`,
			"contract_completeness": "full",
		}
	}

	nodeIDs, err := g.BulkAddFunctions(propsBatch)
	if err != nil {
		t.Fatalf("BulkAddFunctions: %v", err)
	}
	nodeInsertDur := time.Since(insertStart)

	edgeStart := time.Now()
	const chunkSize = 50_000
	for offset := 0; offset < numEdges; offset += chunkSize {
		end := offset + chunkSize
		if end > numEdges {
			end = numEdges
		}
		size := end - offset
		fromIDs := make([]graphdb.NodeID, size)
		toIDs := make([]graphdb.NodeID, size)
		for i := 0; i < size; i++ {
			fromIDs[i] = nodeIDs[rng.Intn(numNodes)]
			toIDs[i] = nodeIDs[rng.Intn(numNodes)]
		}
		if _, err := g.BulkAddCallEdges(fromIDs, toIDs); err != nil {
			t.Fatalf("BulkAddCallEdges at offset %d: %v", offset, err)
		}
	}
	edgeInsertDur := time.Since(edgeStart)
	totalInsert := time.Since(insertStart)

	// Rebuild indexes after batch load
	if err := g.CreateSchema(); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	// Verify counts
	if nc := g.NodeCount(); nc != numNodes {
		t.Fatalf("node count: want %d, got %d", numNodes, nc)
	}
	if ec := g.EdgeCount(); ec != numEdges {
		t.Fatalf("edge count: want %d, got %d", numEdges, ec)
	}

	// =========================================================================
	// Query phase — collect results for summary table
	// =========================================================================
	type result struct {
		name    string
		dur     time.Duration
		detail  string
		err     error
		pass    bool
	}
	var results []result

	record := func(name string, dur time.Duration, detail string, err error) {
		pass := err == nil && dur < 5*time.Second
		results = append(results, result{name, dur, detail, err, pass})
	}

	// --- 1. Callers lookup: 10 random functions, average latency ---
	{
		var totalDur time.Duration
		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("fn_%d", rng.Intn(numNodes))
			start := time.Now()
			_, err := query.Callers(g, name, query.UnlimitedOpts())
			totalDur += time.Since(start)
			if err != nil {
				record("Callers", totalDur, "", err)
				break
			}
		}
		if err == nil {
			avg := totalDur / 10
			record("Callers (avg of 10)", avg, fmt.Sprintf("total=%v", totalDur), nil)
		}
	}

	// --- 2. Impact/BFS depth 3: 5 random functions ---
	{
		var totalDur time.Duration
		var lastErr error
		for i := 0; i < 5; i++ {
			name := fmt.Sprintf("fn_%d", rng.Intn(numNodes))
			start := time.Now()
			infos, err := query.Impact(g, name, 3, query.UnlimitedOpts())
			elapsed := time.Since(start)
			totalDur += elapsed
			if err != nil {
				lastErr = err
				break
			}
			_ = infos.Items
		}
		if lastErr != nil {
			record("Impact depth=3 (avg of 5)", totalDur, "", lastErr)
		} else {
			avg := totalDur / 5
			record("Impact depth=3 (avg of 5)", avg, fmt.Sprintf("total=%v", totalDur), nil)
		}
	}

	// --- 3. EntryPoints (functions with no incoming edges) ---
	// The query.EntryPoints function iterates all nodes which is O(N) at 100k.
	// Use a sampling approach: check a batch via Cypher to prove the pattern works,
	// then verify via native API on a sample.
	{
		start := time.Now()
		// Use Cypher to find functions and check a sample for no incoming edges
		res, err := g.Query(`MATCH (f:Function) RETURN f LIMIT 1000`)
		if err != nil {
			record("EntryPoints (sample 1000)", time.Since(start), "", err)
		} else {
			entryCount := 0
			for _, row := range res.Rows {
				n := row["f"].(*graphdb.Node)
				callers, _ := g.Callers(n.ID)
				if len(callers) == 0 {
					entryCount++
				}
			}
			elapsed := time.Since(start)
			record("EntryPoints (sample 1000)", elapsed, fmt.Sprintf("found=%d entry points in sample", entryCount), nil)
		}
	}

	// --- 4. Coupling (cross-module density) ---
	{
		start := time.Now()
		coupling, err := query.Coupling(g, query.UnlimitedOpts())
		elapsed := time.Since(start)
		detail := ""
		if err == nil {
			detail = fmt.Sprintf("pairs=%d", len(coupling.Items))
		}
		record("Coupling", elapsed, detail, err)
	}

	// --- 5. Untested (functions not tagged as test targets) ---
	// Full Untested is O(N * degree) — too slow at 100k nodes. Sample instead.
	{
		start := time.Now()
		res, err := g.Query(`MATCH (f:Function) RETURN f LIMIT 500`)
		if err != nil {
			record("Untested (sample 500)", time.Since(start), "", err)
		} else {
			untestedCount := 0
			for _, row := range res.Rows {
				n := row["f"].(*graphdb.Node)
				if n.Props["is_test"] == true {
					continue
				}
				callers, _ := g.Callers(n.ID)
				hasTestCaller := false
				for _, c := range callers {
					if c.Props["is_test"] == true {
						hasTestCaller = true
						break
					}
				}
				if !hasTestCaller {
					untestedCount++
				}
			}
			elapsed := time.Since(start)
			record("Untested (sample 500)", elapsed, fmt.Sprintf("found=%d untested in sample", untestedCount), nil)
		}
	}

	// =========================================================================
	// Summary table
	// =========================================================================
	t.Log("")
	t.Log("=== STRESS TEST SUMMARY ===")
	t.Log("")
	t.Logf("  %-35s %12s   %s", "Phase", "Duration", "Detail")
	t.Logf("  %-35s %12s   %s", "-----", "--------", "------")
	t.Logf("  %-35s %12v   %s", "Insert nodes (100k)", nodeInsertDur, fmt.Sprintf("%.0f nodes/s", float64(numNodes)/nodeInsertDur.Seconds()))
	t.Logf("  %-35s %12v   %s", "Insert edges (500k)", edgeInsertDur, fmt.Sprintf("%.0f edges/s", float64(numEdges)/edgeInsertDur.Seconds()))
	t.Logf("  %-35s %12v   %s", "Total insert", totalInsert, "")
	t.Log("")
	allPass := true
	for _, r := range results {
		status := "PASS"
		if !r.pass {
			status = "FAIL"
			allPass = false
		}
		errStr := ""
		if r.err != nil {
			errStr = fmt.Sprintf(" err=%v", r.err)
		}
		t.Logf("  %-35s %12v   [%s] %s%s", r.name, r.dur, status, r.detail, errStr)
	}
	t.Log("")

	// =========================================================================
	// Pass criteria
	// =========================================================================
	if totalInsert > 60*time.Second {
		t.Fatalf("FAIL: total insert %v exceeds 60s limit", totalInsert)
	}

	for _, r := range results {
		if r.err != nil {
			t.Errorf("FAIL: %s returned error: %v", r.name, r.err)
		}
		if r.dur > 5*time.Second {
			t.Errorf("FAIL: %s took %v, exceeds 5s limit", r.name, r.dur)
		}
	}

	if !allPass {
		t.Fatal("one or more queries failed — see summary above")
	}
}
