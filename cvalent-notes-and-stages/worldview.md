# Valent Worldview

**Status:** Source of truth for product framing and decision principles
**Audience:** Anyone designing, building, or selling Valent

---

## What we believe

Code, data pipelines, and infrastructure are the same thing at different levels of abstraction. A Go function, a Kafka consumer, a dbt model, and a Lambda all take data in, transform it, and put data out. They differ only in which operational questions are variable versus locked. A codebase is a pipeline where the hard operational questions — credentials, runtime, engine, trigger, connector state — have already been answered and are invisible to the developer. A pipeline is a codebase where those questions are still being answered and are everyone's problem.

Real systems are deep and tangled. Tracing a single change end to end — "if I rename this field, what breaks" — requires walking through code in three languages, two pipeline systems, and a database schema, asking different tools different questions and stitching the answers together by hand. LLMs trying to do this today spend most of their tokens on exploration: opening files, reading neighbors, inferring boundaries, recovering context after every compaction. That exploration is what makes agentic workflows expensive and unreliable.

Valent makes this complexity shallow. Instead of every agent re-deriving the structure of the system from raw source on every task, the structure exists as a single typed, versioned, evidence-backed graph the agent can query directly. The agent stops paying for exploration and starts paying for answers. The complexity that used to live in token-expensive traversal moves into a substrate that did the traversal once and remembers.

---

## The application layer is a harness; the harness needs a substrate

The clearest framing of the AI application layer comes from Kishen Patel: an AI application is not a UI with a thin LLM backend. It is a *harness* — an orchestration layer that structures how agentic tasks are executed. Harnesses consist of stages, contracts, hooks, memory systems, and error-mode catalogs. The harness decides which files to retrieve, how to construct prompts, which tools to call, when to retry, when to give up, and how to cache results so the next run is cheaper.

A well-designed harness can make a cheaper model outperform a more expensive one. A poorly designed harness wastes tokens on every task. The advantage of the application layer over the model layer comes from three structural properties: (1) the harness can minimize tokens per outcome, aligning with enterprise incentives instead of model-company incentives; (2) the harness can route across models, exploiting differences no single model company can replicate; (3) the harness can evolve from execution signal, accumulating proprietary knowledge about specific workflows in a way that transfers across model providers.

Valent is not a harness. Valent is the substrate that harnesses consume. Every harness — Claude Code, Cursor, Cognition, an enterprise's in-house LangGraph orchestrator, a future first-party Valent harness — needs the same things: a typed model of the customer's code and infrastructure, a memory of how things have changed over time, an evidence trail of what has actually happened when systems run, and a way to ask "if I do X, what breaks." Today every harness builds these from scratch per customer, badly. Valent gives any harness those primitives via MCP, and the harness becomes more efficient, more reliable, and more durable than it could be alone.

The model companies will not build this. Their incentive is to sell tokens, not to make tokens go further. The harness companies will not build this on their own either, because they are competing on the harness layer, not the substrate, and rebuilding the substrate per customer is exactly the work they need to stop doing.

Valent is the durable substrate underneath the harness layer. First-party harnesses that ride on top of Valent are downstream products, not the core. They prove what the substrate enables; they do not replace it.

---

## The three verbs

The value the substrate delivers to its consumers — humans and agents alike — reduces to three verbs in order:

1. **See.** The agent can ask "what exists in this organization, what does it promise, and what depends on what" and get a concrete, current answer from the substrate. Today this answer requires reading dozens of files and inferring relationships. With Valent, it is one query.

2. **Reduce context.** The agent pays for *answers*, not for *exploration*. A query like "what is the blast radius of changing this function" returns a ranked, scoped list — not a hint that the agent then has to expand by reading source. Every byte returned is signal. The agent's context window is spent on the work, not the lookup.

3. **Change with confidence.** Because the substrate carries contracts, guarantees, history, and execution evidence, an agent (or a human) can propose a change and know the actual impact before acting. This is the property that makes agentic editing of large systems safe enough to automate.

The order matters. *See* is the precondition for *reduce context*, which is the precondition for *change with confidence*. A substrate that only delivers *see* is a fancy code search. A substrate that delivers *see* and *reduce context* is an efficient code intelligence layer. Only one that delivers all three is the substrate the application layer actually needs.

The single load-bearing word in the value prop is **shallow**. We are inverting Ousterhout's "deep modules" idea onto organizational complexity. Real organizations are deep — many layers of abstraction, many systems, many languages, many runtimes — and that depth taxes every agent that tries to navigate them. Valent's job is to make the depth shallow at the surface the agent sees. The depth is still there in the underlying systems; the substrate hides it behind a query interface that returns answers, not hints.

---

## The cross-language invariant

A call across a language or runtime boundary is never a direct edge between two function nodes. It always passes through a third node that represents the wire format — a table, a stream, a bucket, an endpoint, a pipeline step. Go does not call Python. Go writes to a Kafka topic; Python reads from the same topic. The boundary is always materialized as a node.

This invariant is what makes cross-language identity a non-problem. Functions never share identity across languages. They share *edges* to boundary nodes. The Go function and the Python function are distinct nodes. The Kafka topic between them is the node they both touch. Identity within a language is keyed on the language-native distribution (`go.mod` module, Maven coordinates, `package.json` name, Python distribution name). Identity for boundary nodes is keyed on the OpenLineage namespace and name conventions, which are already standard.

This invariant also tells you why code parsing alone cannot build a serious infrastructure picture. Code parsers are good at what is in the code — function signatures, calls, contracts on types. They are not good at extracting infrastructure references reliably (config files, environment variables, runtime initialization patterns). Infrastructure is the domain of runtime ingestion: OpenLineage events, native runtime SDKs, declared configuration. Where code-side and runtime-side observations overlap, runtime wins on identity, and code-side observations contribute as best-effort enrichment marked with `Source: parsed` so the agent can weight them appropriately.

Stated as a design principle:

> Code parsers are the source of truth for code structure (functions, methods, calls, types, contracts on types). Runtime ingestion (OpenLineage, native SDKs) is the source of truth for infrastructure (storage, pipelines, endpoints, traces). Where they overlap, runtime ingestion wins on identity, and code parsing contributes best-effort enrichment. The substrate stores both and exposes both; it does not pretend code parsing can substitute for runtime evidence.

---

## The product line

Valent is one product expressed at two layers, both delivering the same substrate:

**OSS (the trailhead).** Free, MIT-licensed, single-repo, locally-installed, zero infrastructure. Parses a codebase, builds the function-level graph in a local SQLite-backed store, and exposes it via a stdio MCP server. Fully featured for what it does — there is no deliberate crippling of the OSS to force upgrades. The OSS is the funnel, the demo, and the distribution mechanism.

The OSS is *engineered to expose its own limits.* Every cross-repo reference, every external import, every contract impact that escapes the local graph is annotated in query results with the resolved external identity (per Model B) and a plain-text note that the hosted version can answer the cross-repo question. This is how the conversion trigger fires: the user discovers the wall by hitting it, and the wall tells them where the door is.

**Hosted (the mountain).** Paid, cloud, multi-tenant, multi-repo, eventually multi-environment. Stores a unified org-scoped substrate in Postgres (Neon at launch). Receives pushes from any number of OSS CLIs in the same organization. Exposes a remote MCP endpoint that any agent can connect to with an API key. Adds, over time, bitemporal contract history, team sharing, pipeline ingest via OpenLineage, execution trace ingest, and the analysis/feedback loop. Does not replace the OSS; the OSS continues to be the install path and the source of code data, while the hosted store accumulates everything across the organization.

Distribution is *upgrading, not switching*. The OSS CLI is the installer, the auth client, and the upgrade path. A user goes from "I installed the OSS" to "I am paying" in one shell command (`cvalent login && cvalent push`). There is no separate web product to discover, no SDK to adopt, no new mental model to learn.

---

## What we are not building

These exclusions are deliberate and durable. They exist to keep the team focused on what only Valent can do, and to avoid getting drawn into races against entrenched competitors that have already won their respective categories.

- **Not a harness.** First-party harnesses come later, on top of the substrate, as reference implementations and downstream products. We are not Cursor, we are not Cognition, we are not Claude Code. Building a harness at launch puts us in the wrong fight.
- **Not an observability product.** We are not Datadog, Honeycomb, or New Relic. We do not chase logs, metrics, distributed tracing, or APM dashboards. We ingest execution evidence as a feedback mechanism for contracts and guarantees, not as a system-of-record for ops.
- **Not a data quality product.** We are not Monte Carlo, Bigeye, or Soda. The contract+guarantee layer overlaps conceptually but the buyer, the use case, and the integration model are different. We do not compete in the data quality category.
- **Not a code search product.** We are not Sourcegraph. We do not optimize for symbol lookup, regex search, or code review surfaces. We optimize for agent-consumable structural queries.
- **Not a graph database.** We use Postgres. The word "graph" describes our model, not our storage engine. We will not adopt Neo4j, DGraph, or any specialized graph DB; the cost-benefit is wrong for our query patterns and operational reality.
- **Not "DataValent."** There is no separate Valent product for data pipelines. The schema notes once contemplated splitting CodeValent and DataValent into two brands; this is no longer the plan. Valent is one substrate. CodeValent is retained only as the launch trailhead framing — the face of Valent that users see when they first install the OSS.
- **Not a general-purpose data catalog.** We are not Atlan or DataHub. Cataloging is a side effect of the substrate, not the product.
- **Not a pricing-table SaaS competing on feature lists.** We compete on substrate quality and the compounding moat the substrate generates from execution signal over time. Feature lists are downstream of that.

When tempted to build any of the above, the answer is to instead find the *substrate primitive* that would let an existing product in the category be better. Sell that primitive. Stay underneath.

---

## The economics

Per Patel: model companies sell tokens; enterprises buy outcomes. The misalignment is structural and grows as token prices fall, not shrinks. An agent that delivers an outcome for 100 tokens instead of 1,000 does not just save money on existing work; it opens up ten times more use cases at the same ROI threshold. Token efficiency expands the addressable market of tasks worth automating at all.

Valent's value to a harness is measured in tokens-per-outcome saved. A harness consuming Valent should reach the same answer with materially fewer tokens than a harness without it, because the harness no longer has to retrieve and read source code to derive structure. The substrate gives it the structure directly. Every query answered by the substrate is tokens that did not have to be spent on exploration.

This is also why we resist becoming a harness. A first-party harness optimizes one thing well. The substrate underneath optimizes *every* harness that consumes it, including competitors. The leverage is bigger and the moat is durable, because the moat is in the data and the model, not in the harness logic.

The pricing model at launch is intentionally low and undifferentiated — a flat, generous design-partner price for the Hosted Account. The job of launch pricing is to find signal, not to capture margin. Capture comes later, when the substrate has accumulated enough proprietary knowledge of customer systems that switching cost is real.

---

## The moat

The moat is not the schema. The schema is the *enabler* of the moat — the data structure that lets us capture the right things in the right shape. The moat itself is what accumulates inside the schema over time: contracts that have been observed against thousands of executions, guarantees with confidence levels grounded in real evidence, contract violation patterns that recur across customers, shift detections that have been validated by humans, contract proposals that have been accepted and then succeeded in production. Each of these is proprietary knowledge about how data actually flows through this customer's specific systems, in this customer's specific industry, with this customer's specific failure modes.

This is the Meta-Harness feedback loop applied to code and data workflows. Every task an agent runs against the substrate generates signal — which queries it asked, which it found insufficient, which led to successful changes, which led to violations. That signal feeds back into the substrate. Over time the substrate gets better at answering the queries that matter and at flagging the changes that have historically led to incidents. None of this is possible without the substrate as the central evidence store, and none of it happens at all if the substrate is empty or unused.

The moat compounds in two ways. First, *within a customer*: the more the substrate is used, the more evidence accumulates, the more confident its answers become, the harder it is to switch away. Second, *across customers*: patterns that hold up across many customers (a particular kind of contract drift, a particular confidence-degrading sequence) become first-class capabilities of the system itself, available to every customer. Both compounding modes require the same precondition — that the substrate exist, be populated, and be widely consumed. That is what the entire product strategy is organized around.

---

## The development path: rungs, not phases

We previously thought of the product as a sequence of phases: code first, then cloud, then agents, then observability, then extensibility. That framing was wrong because it implied we were building separate things on top of each other. We are not. We are building one substrate and progressively populating it with more sources of information.

The right framing is *rungs*. Each rung is a deployable, valuable increment that adds a new capability to the same substrate. The data model never changes shape across rungs; what changes is which parts of the model are populated, and which features expose them. Six rungs from now to the first complete version of the substrate:

| Rung | Name | What's new | Graph populated by |
|---|---|---|---|
| 0 | Pre-launch baseline | Unified `Node` model, parser identity normalization (Model B), SQLite store, GoraphDB retired, dialect adapter, query parity harness, OSS friction engineering | Code parsers (functions, calls, contracts) |
| 1 | Hosted CodeValent Account | Neon Postgres, Clerk auth, push protocol, remote MCP endpoint, cross-repo single-account store, empty-field direction signals | Code, now centrally stored and cross-repo |
| 2 | Bitemporal history | Pushes stop overwriting, valid_from/valid_until activated, contract_history populates, point-in-time queries work | Code + history of code |
| 3 | Teams and sharing | Multi-user orgs, roles, shared graph views, full Clerk integration | Code + history + multi-user |
| 4 | Pipeline and infrastructure ingest | OpenLineage receiver, PipelineMeta/StorageMeta/EndpointMeta types and tables live, code-side storage reference extraction (best-effort), declared infrastructure path | Code + infra (first rung where infrastructure exists in the graph) |
| 5 | Execution traces and analysis loop | ExecutionTrace ingest, ContractViolation accumulation, guarantee confidence updates, analysis DB live (shifts, proposals, diagnoses) | Everything above + runtime evidence; feedback loop begins |

Beyond Rung 5: first-party harnesses on top of the substrate, the connector library, PII path tracking, cross-environment Arrow IPC enforcement. Treated as "post-5" rather than committed to specific numbering, because their shape depends on what Rung 5 actually reveals.

---

## Decision principles

When in doubt, default to these. They override any locally-optimal choice.

1. **Substrate, not harness.** When tempted to build orchestration, agent loops, or task execution machinery, stop. Build the data primitive that would let any harness do the orchestration better.

2. **One model, populated progressively.** Every feature lands inside the unified `Node` model. Do not invent parallel models for new node kinds. Do not refactor the model to accommodate features; design features to fit the model.

3. **Make it shallow.** When designing a query, an MCP tool, or an API response, ask: does this hand the agent an *answer*, or does it hand the agent another lookup it has to perform? Answers are product. Lookups are databases. We are a product.

4. **Distribution is upgrading.** Every paid feature must be reachable from the OSS install in one shell command. No separate signup flows, no separate installers, no parallel mental models. The OSS user graduates without noticing they moved.

5. **Reliability over completeness.** A small surface that is bit-exactly correct beats a large surface that is mostly right. Trust is the precondition for adoption; once lost it does not return. When in doubt about scope, cut.

6. **Postgres for everything.** When deciding where to put new state, the answer is "a Postgres table." Add new storage engines only with measured evidence that Postgres has run out of room. Same applies on the OSS side with SQLite.

7. **Identity is Model B + OpenLineage.** Never invent a new identity scheme. Code identity is language-native distribution. Boundary identity is OpenLineage namespace and name. The composite key is `(EnvironmentID, Kind, QualifiedName)`. Deviations require an explicit decision recorded in memory.

8. **Empty fields point at the future.** API response shapes commit to the full direction of the product even when the data behind them is not yet populated. An empty `pipeline_references` array tells the user that pipelines are coming. This is the cheapest possible roadmap signal and it costs nothing to maintain.

9. **Source-tagged evidence.** Every fact in the graph carries a `Source` (`parsed`, `openlineage`, `declared`, `discovered`). When sources disagree, runtime wins over parsed, declared wins over discovered, and the agent gets to weigh the difference.

10. **Cut downstream cleanup over upstream cleverness.** When a feature would require complex logic at query time to compensate for missing structure at ingest time, fix the ingest. The query path is the hot path; clever query logic is technical debt with interest.

---

## Cross-references

- Memory: `project_valent_product_shape.md`, `project_valent_value_prop.md`, `project_valent_launch_shape.md`, `project_valent_identity_model.md`, `project_valent_storage_stack.md`
- Schema: `cvalent-notes and stages/schema/valent_schema.md`, `valent_schema_decisions.md`, `valent_graph_schema.md`, `valent_observations_schema.md`, `valent_analysis_schema.md`
- Per-rung detail: `phase0_baseline.md` through `phase5_traces_analysis.md` in this directory
