# Decision: Domain Detection — Deferred, Not Dropped

## Decision
Domain detection (community detection algorithms, cohesion scores, architectural drift) is out of Phase 1. No rework risk.

## Rationale
Phase 1 builds the graph with correct edges (call, import, data flow). Domain detection is a downstream analysis that reads existing edges and writes labels. It layers on clean.

## Sequence
1. **Phase 1**: Build the graph with correct edges. Query coupling on demand via edge-count queries over existing topology.
2. **Later**: Add a `cvalent classify` command (or fold it into `cvalent build`) that runs domain detection over the existing edge data and writes labels back to the graph. Zero rework — it's additive. SQLite schema change is a migration, not a rewrite.

## Key constraint
The only way this becomes rework is if the edge data is wrong or incomplete. M1's job is getting edges right. If the edges are solid, any downstream analysis — domain detection, PII tracing, coupling scores — layers on clean.

## User's goal
"Understand the next layer of the code" — are auth components scattered? What naturally groups together? This is implicit architecture discovery: where *should* code live based on what it does and depends on, not where someone put the file.
