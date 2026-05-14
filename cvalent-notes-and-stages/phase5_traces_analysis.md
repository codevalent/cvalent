# Rung 5 — Execution Traces and Analysis Loop

**Status:** The rung that turns the substrate into a learning system. The first version of the moat compounds here.
**Position:** Rung 5 of 5 in the launch path; everything beyond is "post-5"
**Depends on:** Rung 4 (infrastructure must be in the graph for traces to attach to anything), Rung 2 (history is what makes a trace useful — a trace against a frozen graph is just a log)
**Unlocks:** First-party harnesses on top of the substrate, the connector library, PII path tracking, cross-environment Arrow IPC enforcement — all "post-5"

---

## Goal

Start ingesting execution evidence — traces, contract violations, runtime guarantees — and feed it back into the substrate so that contracts get confidence levels, guarantees get grounded in real observations, and the analysis DB starts producing shift detections, contract proposals, and incident diagnoses. This is the rung where the Meta-Harness feedback loop from Patel's essay actually fires inside Valent.

This is the rung where the substrate stops being a static structural map and starts being a living, evidence-backed model of how the customer's systems actually run.

---

## Why this is its own rung

This rung is by far the largest in scope and the riskiest in execution. Three reasons it sits last.

First, there is nothing useful to feed traces *into* until Rung 4 is real. A trace against a function node with no upstream/downstream pipeline context is a log line; a trace against a function node connected to its Kafka topic and the downstream Python consumer is a piece of evidence about an end-to-end path. Rung 5 needs Rung 4's infrastructure to mean anything.

Second, the analysis DB is the most schema-heavy and most product-design-heavy part of the entire substrate. Shift detection, contract proposals, guarantee confidence, incident diagnoses — every one of these has its own pending → accepted/rejected/applied lifecycle and its own UX implications. Doing this on top of a stable, paying, multi-rung-deep substrate means there are real customers to validate against and real signal to tune against. Doing it earlier would have meant designing in the dark.

Third, this is the rung where the moat stops being structural and starts being compounding. Earlier rungs compete on substrate *quality*; this rung competes on substrate *learning rate*. That is the deeper, more durable advantage, but it only matters once everything below it is reliable enough that the learning is grounded in real signal.

---

## What changes for the user

- **`recent_traces` and `contract_violations` fields populate.** The empty arrays from Rung 1 begin returning real entries scoped to whatever the user is querying.
- **Guarantees gain confidence levels.** Where Rung 4 had "this contract claims X," Rung 5 has "this contract claims X and the runtime evidence says X holds in 99.7% of observed executions." The confidence is grounded in trace counts, not in vibes.
- **Contract violations.** When runtime evidence disagrees with a stated contract, a `ContractViolation` row is recorded. The user can ask "what contracts are being violated right now" and get a real answer.
- **Shift detection.** A background analyzer watches for distribution shifts in observed values flowing through edges (a column whose value distribution changed, a topic whose throughput changed shape, a pipeline whose runtime jumped). Shifts are recorded as `ShiftDetection` rows in the analysis schema and surfaced as a new MCP tool.
- **Contract proposals.** When the analyzer notices a recurring undeclared invariant (a field that has always been non-null, a value that has always been within a range), it proposes a contract addition. The user (or an agent on their behalf) can accept, reject, or amend. Accepted proposals become real contracts.
- **Guarantee proposals.** Same lifecycle, for guarantees.
- **Incident diagnoses.** When a contract violation correlates in time with a deploy that changed the relevant code, the analyzer produces a candidate diagnosis tying the violation to the change. The diagnosis has a confidence level and an evidence trail.
- **The first compounding answer.** "What is the blast radius of changing this function" stops being a static graph traversal and starts including a runtime-evidence weighting: "this edge is exercised 10,000 times per hour with this contract; that edge is unexercised in the last 30 days." The agent gets answers that are not just structurally correct but operationally calibrated.

---

## What gets built

**Trace ingestion endpoint.** A high-throughput endpoint (probably its own service, separate from the OpenLineage receiver and the push protocol) that accepts execution trace events. Wire format reuses OpenLineage runEvents where possible to avoid inventing a parallel schema. Backed by Postgres time-partitioned tables in the `obs` schema.

**`ExecutionTrace` table populated.** Per the existing observations schema. Partitioned by month for retention management.

**`ContractViolation` accumulator.** A streaming process that joins incoming traces against current contracts and records violations. Backed by `obs.contract_violations`. Has a backpressure / batch-flush mode for high-volume customers.

**Guarantee confidence updater.** A periodic (or streaming) job that updates `guarantee_confidence_history` based on accumulated trace volume. The current confidence is materialized into the `guarantees` table for query speed.

**Analysis DB live.** All the tables in `valent_analysis_schema.md` get their first writes:
- `shift_detections` from the shift analyzer
- `contract_proposals` from the proposal analyzer
- `guarantee_proposals` from the same
- `incident_diagnoses` from the deploy-correlation analyzer
- `impact_assessments` from agent queries that ask "if I do X, what breaks"
- `trend_analyses` from rolling-window aggregators

**Analyzers.** Concrete analyzer implementations for each of the above. Initially crude — a single thresholded statistic per analyzer is enough to start producing signal. The analyzers improve over time as the cross-customer signal accumulates.

**Pending → accepted/rejected/applied lifecycle.** Every analysis output starts as `pending`. A user (or an authorized agent) moves it through the lifecycle. Applied proposals mutate the substrate; rejected proposals are kept for negative signal; accepted-but-not-yet-applied proposals are visible but not active.

**Feedback loop into the substrate.** This is the load-bearing piece. When a proposal is applied, the substrate gets a new contract (or guarantee, or fix). When a proposal is rejected, the rejection is signal for the analyzer to suppress similar proposals. When a diagnosis is confirmed, the diagnosis enters the cross-customer pattern library. Every loop closure makes the next loop better.

**MCP tool surface for analysis.** New tools:
- `recent_violations(node_id?, since?, until?)`
- `pending_proposals(kind?, node_id?)`
- `apply_proposal(proposal_id)` / `reject_proposal(proposal_id)`
- `diagnose_incident(symptom_query)` — backed by the diagnosis analyzer
- `runtime_weighted_impact(function_id)` — the structurally + operationally weighted blast radius
- `confidence_for(contract_id | guarantee_id)`

**Cross-customer pattern library (private).** Patterns that recur across many customers (a particular kind of contract drift, a particular failure mode signature) get promoted into a first-class capability of the system. This is the second compounding mode of the moat. It is private — no customer can see another customer's specifics — but every customer benefits.

**Sampling and cost controls.** Trace ingest is the highest-volume surface in the product. Per-account sampling, per-edge sampling, and adaptive throttling all live in this rung. The analysis DB also has retention policies because shift detections from a year ago are mostly noise.

**Pricing dimension (likely).** This is the rung where usage-based pricing becomes hard to avoid. Trace volume is the most natural metric. Decide late, instrument early.

---

## What gets populated in the graph and adjacent stores

- `obs.execution_traces` (high volume)
- `obs.contract_violations`
- `obs.guarantee_confidence_history`
- `obs.edge_compatibility_changes`
- `obs.gap_detection_events`
- All `analysis.*` tables
- `recent_traces` and `contract_violations` direction-signal fields populated on responses

The `graph` schema gains very little new content at this rung — what changes is that its existing contents are now backed by evidence and weighted by confidence.

---

## What does NOT get populated (left for post-5)

- First-party harnesses on top of the substrate
- The connector library (turn-key emitters for common runtimes that don't speak OpenLineage natively)
- PII path tracking
- Cross-environment Arrow IPC enforcement
- Full cross-environment temporal alignment (only partial alignment at this rung)

These are post-5 because their shape depends on what Rung 5 actually reveals. Committing to them now is committing in the dark.

---

## MCP tool surface changes

Six new tools listed above. Every existing impact / blast-radius tool gains an optional `weight_by_runtime: true` parameter that pulls the runtime-weighted variant. The default remains the structural variant for predictability; agents opt in to weighting where they want it.

---

## Conversion trigger

This rung is the first one whose value compounds without the user doing additional work after onboarding. Once trace ingest is configured, every execution makes the substrate smarter. The conversion narrative for new customers becomes: "every previous tool is a static map; this is the only one that learns from how your systems actually run."

For existing customers: this is the rung where retention becomes structural. Every accepted proposal, every confirmed diagnosis, every confidence-adjusted guarantee is proprietary signal that does not exist anywhere else. Switching cost stops being theoretical.

---

## Definition of done

- [ ] Trace ingest endpoint live; sustained throughput meets at least one paying customer's real workload
- [ ] `obs.execution_traces` and `obs.contract_violations` populated and queryable
- [ ] Guarantee confidence levels updated continuously and materialized for query
- [ ] All `analysis.*` tables receiving writes from at least one analyzer each
- [ ] At least one shift detection, one contract proposal, and one incident diagnosis surfaced against real customer data
- [ ] Pending → accepted/rejected/applied lifecycle implemented end-to-end
- [ ] Feedback loop demonstrably closes: an applied proposal mutates the substrate, a subsequent query reflects the change
- [ ] Six new MCP tools shipped and documented
- [ ] Sampling and cost controls in place; one runaway emitter cannot melt the system
- [ ] Cross-customer pattern library has at least one promoted pattern
- [ ] At least one paying customer is using the runtime-weighted blast radius for a real change decision
- [ ] Documentation: full trace onboarding flow, proposal lifecycle, analysis tools

---

## Dependencies on prior rungs

- **Rung 4 infrastructure ingest** — traces need somewhere to attach; without infra nodes they're just timestamps
- **Rung 2 history** — every trace is implicitly bitemporal (it observed a graph that was valid at a past time); without Rung 2, attribution is wrong
- **Rung 1 hosted store** — none of this works on local SQLite
- **Rung 0 identity** — traces from runtime carry identity; if code-side identity disagrees, joins fail

---

## Open questions deferred

- Whether the analysis DB should ever be exposed for direct customer SQL access (probably no; revisit)
- Per-customer model fine-tuning on their own analysis history (probably never; the harness layer does this, not the substrate)
- pgvector across traces for similarity search (live opportunity at this rung; ship if measured to help)
- How to attribute violations to deploys when deploy events come from yet another source (probably another Rung 4–style ingest path; design at the time)
- The full shape of cross-customer pattern promotion — what becomes a pattern, who reviews, how it's pushed back to customers — needs real signal to design against, not speculation

---

## Cross-references

- Worldview: § "The moat", § "The economics", § "Decision principles" #9 (Source-tagged evidence)
- Memory: `project_valent_storage_stack.md` (Postgres for `obs` and `analysis` schemas)
- Schema: `valent_observations_schema.md`, `valent_analysis_schema.md`
