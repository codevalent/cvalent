# Rung 4 — Pipeline and Infrastructure Ingest

**Status:** First rung where the substrate covers infrastructure, not just code. The biggest scope expansion in the product line.
**Position:** Rung 4 of 5
**Depends on:** Rung 1 (hosted store), Rung 2 (history) — Rung 3 not strictly required but assumed shipped
**Unlocks:** Rung 5 (execution traces and analysis loop) — traces against an empty infrastructure graph are useless

---

## Goal

Light up the parts of the unified `Node` model that have always been defined but never populated: pipelines, storage, endpoints, and the cross-boundary edges connecting code to infrastructure. Receive OpenLineage events from any compatible runtime (dbt, Airflow, Spark, Dagster, custom emitters). Best-effort enrich with code-side storage references parsed from source. Surface the third-node invariant — a Go function writing to a Kafka topic that a Python consumer reads — as a real, queryable path through the substrate.

This is the rung where Valent stops being "code intelligence" and starts being "code + data + infrastructure intelligence." The schema has always promised this; this is the rung that ships it.

---

## Why this is its own rung

Three reasons.

First, scope. This rung doubles the surface area of the substrate. New node kinds, new identity rules (OpenLineage namespaces), new ingestion path (OpenLineage receiver, separate from the push protocol), new edges, new query patterns. Bundling it earlier would have made every earlier rung more expensive without proportional value, because none of those rungs need pipeline data to deliver their value.

Second, identity. The Model B work in Rung 0 only resolved code-side identity. This rung is where storage/boundary identity (OpenLineage) is exercised end-to-end for the first time. Doing it on top of a stable, paying, history-aware substrate means the identity work has real users to verify against, not a green field to speculate at.

Third, the conversation it opens. Selling "code graph" (Rungs 1–3) is selling against Sourcegraph and the harness companies. Selling "code + pipelines + infrastructure as one substrate" is selling against nothing — there is no incumbent in the cross-domain shape. This rung is where the moat becomes structurally defensible because it occupies a category nobody else is in.

---

## What changes for the user

- **OpenLineage endpoint to point at.** Any runtime that already emits OpenLineage events (dbt with `dbt-openlineage`, Airflow with the OpenLineage provider, Spark, Dagster, etc.) can be pointed at the Valent ingest URL with an API key. Events flow in. Pipeline nodes appear in the graph. The user does no per-tool integration work — OpenLineage is the universal interface.
- **Declared infrastructure path.** For runtimes that don't emit OpenLineage, the user can declare infrastructure in a small YAML or Go config that the OSS picks up at parse time. Manually declared nodes carry `Source: declared` so the agent can weight them appropriately.
- **Code-side enrichment.** The OSS parsers gain best-effort extraction of storage references (a Go function calling `kafka.NewWriter("topic-x")`, a Python function calling `bigquery.Client().query(...)`). These produce edges from `function` nodes to `storage`/`endpoint` nodes, tagged `Source: parsed`. They fill in code-side coverage where runtime emission isn't happening yet. Where parsed and runtime observations overlap, runtime wins on identity per the principle from the worldview.
- **Cross-language traversal becomes real.** A query like "what reads from this Kafka topic" now returns Python and Go consumers from different repos in the same account. A query like "what is the blast radius of changing this dbt model" returns the downstream SQL consumers and the Go services that read the table the model writes. This is the first rung where the three verbs deliver against infrastructure, not just code.
- **`pipeline_references`, `upstream_storage`, `downstream_storage` direction-signal fields populate.** The empty arrays from Rung 1 become real lists with real entries.

---

## What gets built

**OpenLineage receiver service.** A separate ingest endpoint (probably `lineage.valent.dev`) that accepts OpenLineage events over HTTP, validates them, and writes them into the substrate. Stateless ingest workers; backed by Postgres. Events are normalized through the same `Node` model the rest of the substrate uses — `PipelineMeta`, `StorageMeta`, `EndpointMeta` slots populated.

**`PipelineMeta`, `StorageMeta`, `EndpointMeta` types and tables live.** The schema has always defined these slots; this is the rung where the tables get populated and indexed for query. Identity is `(EnvironmentID, Kind, QualifiedName)` where `EnvironmentID` is the OpenLineage namespace and `QualifiedName` is the OpenLineage name.

**New edge kinds.**
- `reads_from(function | pipeline_step → storage)`
- `writes_to(function | pipeline_step → storage)`
- `calls(function → endpoint)` and `serves(endpoint → function)`
- `derives_from(pipeline_step → pipeline_step)` for dbt-style lineage chains

These all live in the same `edges` table; the kind discriminator handles routing.

**Code-side storage reference extraction (best-effort, opt-in per language).**
- Go: detect `kafka.NewWriter`, `kafka.NewReader`, `bigquery.Client`, `s3.Client.PutObject`, etc., via tree-sitter pattern matching against a known table of stdlib/third-party SDK signatures.
- Python: detect `bigquery.Client`, `boto3.client('s3')`, common Kafka client patterns.
- Java: similar against the JVM ecosystem.
- TypeScript: similar against the JS ecosystem.
- Every parsed storage reference produces an edge tagged `Source: parsed`. The OSS friction engine no longer fires on these (the OSS now has a partial answer); it does fire when the parsed answer is incomplete and the hosted version has runtime coverage.

**Declared infrastructure path.** A `valent.yaml` (or similar) at the repo root that the OSS reads on parse, declaring storage, pipelines, endpoints in a small DSL. Mostly for users whose runtimes don't emit OpenLineage and who don't want to wait for code-side extraction to cover their stack. Declared nodes are tagged `Source: declared`.

**Source precedence resolver.** When the same logical node is observed from multiple sources, the resolver applies: `openlineage > declared > parsed > discovered`. Each version of the fact is preserved in history; the "current" view is the highest-precedence source. Agents can request the full source list via a new MCP tool if they need to disagree with the resolver.

**MCP tool additions.**
- `pipelines_for(node_id)` — pipeline steps that touch this node
- `storage_for(function_id)` — storage this function reads or writes
- `consumers_of(storage_id)` — code and pipeline steps that read this storage
- `producers_of(storage_id)` — code and pipeline steps that write this storage
- `cross_boundary_path(from_id, to_id)` — the third-node-aware path between two code nodes that are connected via infrastructure
- All existing impact / blast radius queries gain awareness of the new edge kinds; this is automatic from the schema.

**Pricing dimension added (optional at this rung).** Pipeline ingest may justify a usage-based component (event volume) on top of the per-seat pricing. Decide based on Rung 1–3 signal; default is to keep launch pricing flat and revisit only if costs force it.

---

## What gets populated in the graph

Everything from Rungs 0–3, plus:
- `pipeline_step` nodes (one per dbt model, Airflow task, Spark job, etc.)
- `storage` nodes (one per Kafka topic, BigQuery table, S3 bucket, Postgres table, etc.)
- `endpoint` nodes (one per HTTP endpoint, gRPC method, webhook, etc.)
- `reads_from`, `writes_to`, `calls`, `serves`, `derives_from` edges
- All of the above with `Source` tagging
- All of the above with bitemporal versioning (Rung 2 makes this free)

---

## What does NOT get populated

- `ExecutionTrace` records (Rung 5)
- `ContractViolation` records (Rung 5)
- Anything in the `analysis` schema (Rung 5)
- Cross-environment Arrow IPC enforcement (post-5)
- PII path tracking (post-5)

---

## MCP tool surface changes

Five new tools listed above. All existing tools gain awareness of the new edge kinds where their semantics overlap (impact, blast radius, callers-of-storage). The `pipeline_references`, `upstream_storage`, `downstream_storage` direction-signal fields begin returning non-empty values.

A new "source filter" parameter on every tool: callers can ask "show me only `Source: openlineage` evidence" or "include parsed enrichment." Default is the resolved view.

---

## Conversion trigger

This rung is the largest single conversion trigger after Rung 1 itself. It doesn't pull from OSS — it pulls from existing customers expanding their substrate to cover their pipelines, and from new customers who are evaluating Valent against the cross-domain category nobody else is in.

For OSS users, friction annotations now also fire on storage references that the OSS parsed locally but couldn't connect to a real downstream consumer ("the hosted version connects this Kafka topic to the Python service that reads it").

---

## Definition of done

- [ ] OpenLineage receiver live, accepting events from at least three runtimes (dbt, Airflow, one of Spark/Dagster) end-to-end
- [ ] `pipeline_step`, `storage`, `endpoint` node kinds populated and indexed
- [ ] All five new edge kinds present in the `edges` table with appropriate indexes
- [ ] Code-side storage reference extraction in all four parsers, opt-in via config
- [ ] Declared infrastructure path: `valent.yaml` parsed, declared nodes tagged correctly
- [ ] Source precedence resolver implemented; tested against constructed conflicts
- [ ] Five new MCP tools shipped and documented
- [ ] All existing impact/blast-radius tools traverse the new edge kinds
- [ ] Direction-signal fields populated where data exists
- [ ] At least one paying customer has connected at least one OpenLineage-emitting runtime and run a cross-boundary query that returned a real path
- [ ] Documentation updated end-to-end including the OpenLineage onboarding flow

---

## Dependencies on prior rungs

- **Rung 0 Model B identity** — needed for the code side of every cross-boundary edge
- **Rung 1 hosted store** — pipeline ingest is a hosted-only feature
- **Rung 2 history** — every pipeline event is a temporal observation; without Rung 2, every event would overwrite

---

## Open questions deferred

- Cross-environment time alignment when OpenLineage events arrive with skewed clocks
- Whether OpenLineage events should be normalized into the same `Node` table or a separate staging area first
- How aggressively to cache the source precedence resolver's output
- Per-namespace permissions (probably handled by Rung 3's org-level model; revisit if a customer asks)
- pgvector / semantic search across the now-much-larger graph

---

## Cross-references

- Worldview: § "The cross-language invariant", § "Decision principles" #7 (Identity is Model B + OpenLineage), #9 (Source-tagged evidence)
- Memory: `project_valent_identity_model.md` (the identity table this rung exercises end-to-end)
- Schema: `valent_schema.md`, `valent_graph_schema.md`
