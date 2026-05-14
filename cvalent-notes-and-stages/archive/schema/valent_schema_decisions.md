# Valent Schema — Design Decisions & Open Questions
**Version**: 0.1  
**Status**: Draft  
**Date**: April 2026

This document records the reasoning behind the schema design choices and the questions that remain unresolved. It is a companion to `valent_schema.md`.

---

## Core Insight

The schema is built on one insight: **a Go function, a Kafka consumer, a dbt model, and a Lambda are all the same thing at different levels of abstraction**. They all take data in, do something to it, and put data out. They differ only in which operational questions are variable versus locked.

A codebase is a pipeline where the hard operational questions have already been answered:
- **Credentials** — handled by the runtime, invisible to the developer
- **Run environment** — the machine the code runs on, stable
- **Engine** — the language, fixed per file
- **Trigger** — a function call, the simplest possible trigger
- **Connector state** — always locked, because the language runtime is the connector

This means CodeValent is not a simplified version of a pipeline tool. It is a pipeline tool where the infrastructure layer is so stable it becomes invisible. The same schema models both — code nodes just have most fields pre-filled and locked.

---

## Decision Log

### D1: One node type with typed metadata slots

**Decision**: All nodes share a common core. Kind-specific fields live in a typed metadata slot (`FunctionMeta`, `PipelineMeta`, etc.). Only one slot is populated per node.

**Reasoning**: This gives the agent a consistent traversal model — it always reads the same fields for contract, state, trigger, and engine regardless of kind. Kind-specific details are available when needed but don't pollute the common model.

**Alternative considered**: Separate types per kind (e.g., `FunctionNode`, `PipelineNode`). Rejected because it would require the agent to handle multiple types during traversal and would make cross-kind queries awkward.

**Consequence**: Adding a new kind requires a new meta type and a validation rule. Nothing else in the system changes.

---

### D2: Node identity is a composite key

**Decision**: A node is uniquely identified by `(EnvironmentID, Kind, QualifiedName)`. The UUID `ID` is derived deterministically from this composite.

**Reasoning**: Rebuilding the graph from source should produce the same node IDs. If IDs were random UUIDs assigned at insert time, every rebuild would break all edges and traces. Deterministic IDs mean the graph is always rebuildable from source without losing history.

**Consequence**: Two nodes with the same qualified name in different environments are different nodes. This is correct — a `ProcessOrder` function in the production codebase and a `ProcessOrder` function in a staging codebase are different nodes with potentially different contracts.

---

### D3: Guarantees have a When dimension

**Decision**: Every guarantee declares when it applies — `precondition` (before execution), `postcondition` (after execution), or `invariant` (always true).

**Reasoning**: Without this dimension, you cannot distinguish "amount must be positive before I run" (a precondition — the caller's responsibility) from "balance is non-negative after I run" (a postcondition — the node's responsibility). This distinction is fundamental to root cause analysis: a precondition violation means the upstream caller is at fault; a postcondition violation means the node itself is at fault.

**Source**: Design by Contract (Bertrand Meyer, 1986). The software world has used this model for 40 years. The pipeline world uses it implicitly but doesn't name it.

---

### D4: Declared guarantees are hypotheses; discovered guarantees are conclusions

**Decision**: `GuaranteeSource` distinguishes `declared` (stated by a developer) from `discovered` (promoted from execution evidence). Declared guarantees start with high confidence by assertion. Discovered guarantees accumulate confidence from observations.

**Reasoning**: This maps directly onto the connector research methodology — you declare what you expect, run controlled experiments, and promote patterns to the library only after sufficient evidence. The same loop applies here. A declared guarantee that keeps failing is a bug. A discovered guarantee with 10,000 observations is more trustworthy than a declared one with 10.

**Consequence**: The system never treats these as the same thing. An agent diagnosing a failure should weight a declared guarantee with 3 violations differently from a discovered guarantee with 3 violations out of 10,000 observations.

---

### D5: Credentials are references, never values

**Decision**: `PipelineMeta.Credentials` is a `CredentialRef` — a provider name and a key path. The actual credential value is never stored in the graph.

**Reasoning**: The graph will eventually be queryable by agents. Storing credentials in it would be a security disaster. The reference is sufficient for an agent to know that credentials exist and where to find them, without the graph itself being a credential store.

**Consequence**: Any system that needs to execute a pipeline step must resolve the credential reference at runtime through the appropriate secrets provider. The graph is never a source of truth for credential values.

---

### D6: EdgeCompatibility is a property of the edge, not computed on demand

**Decision**: When a contract changes, `EdgeCompatibility` is updated on all affected edges immediately. The current compatibility state is always readable without a traversal.

**Reasoning**: The agent needs to answer "show me all breaking edges" efficiently. If compatibility were computed on demand, this query would require comparing contracts on every edge endpoint — O(edges) work. With compatibility as an edge property, it is a direct lookup.

**Consequence**: When a contract changes, a job must run to update compatibility on all edges touching the changed node. This is an explicit maintenance cost in exchange for fast query performance.

---

### D7: OpenLineage compatibility is additive, not structural

**Decision**: This schema does not replace OpenLineage. It is a superset. OpenLineage events from external tools are received, mapped to nodes and traces, and the Valent contract layer is added on top using custom facets in the `valent:` namespace.

**Reasoning**: The goal is to graft onto the existing ecosystem, not replace it. Airflow, Spark, dbt, and Dagster users should not have to change anything. They get the contract layer on top of what they already have. Teams that want to build natively on the Valent platform get full contracts from day one.

**Consequence**: The system must maintain an OpenLineage ingestion pipeline. Incoming events are mapped to nodes and traces according to the OpenLineage-to-Valent mapping defined in the schema document. Custom `valent:` facets are ignored by OpenLineage-compatible tools and processed by the Valent system.

---

### D8: Storage nodes are first-class nodes, not just metadata

**Decision**: A Kafka topic, a Postgres table, and an S3 bucket are nodes in the graph with their own contracts — not just metadata on edges.

**Reasoning**: Storage nodes have contracts too. A Kafka topic promises a schema. A Postgres table promises referential integrity. A S3 bucket promises a file format. These contracts can be violated. When they are, you need to trace the violation through the graph. If storage is only edge metadata, you cannot traverse through it — the causal chain breaks at the storage boundary.

**Consequence**: Storage nodes participate in blast radius analysis. Changing the schema of a Kafka topic has a blast radius just like changing the signature of a Go function. The graph makes this visible.

---

### D9: CodeValent's FunctionNode maps cleanly to this schema

**Decision**: The existing `FunctionNode` in CodeValent becomes a `Node` with `Kind: "function"` and the existing fields relocated to `FunctionMeta`. No information is lost. No rework is required.

**Reasoning**: The common core (`Contract`, `Engine`, `Trigger`, `State`) captures everything that was implicit in the old model. The `FunctionMeta` slot captures everything that was explicit. The current `Parameters` and `Returns` become `Contract.Inputs` and `Contract.Outputs`. The `ContractCompleteness` field maps directly.

**Migration path**: 
1. Add the new schema types alongside existing types
2. Write a migration function: `FunctionNode → Node`
3. Run the migration on existing graph data
4. Remove the old types

---

### D10: The meta-agent reads TraceContext selectively

**Decision**: `TraceContext` stores raw logs, error messages, stack traces, and external references. It is not summarized or compressed.

**Reasoning**: This is the direct application of the Meta-Harness insight. The meta-agent's ability to diagnose complex failures depends on having access to the full causal chain — not a summary of it. Summaries lose the specific line of code that failed, the exact value that violated the constraint, the sequence of events that led to the failure. The agent reads what it needs via selective access, just as Meta-Harness reads prior run logs via `grep` and `cat`.

**Consequence**: `TraceContext` can be large. Storage must handle variable-size text fields efficiently. Old traces may need to be archived or pruned after a retention window. The `ExternalURL` field allows the agent to fetch additional context from source systems when the stored context is insufficient.

---

### D11: Contract is optional

**Decision**: `Node.Contract` is a pointer (`*Contract`). A node can exist without a contract. Absence is surfaced as a `ContractGap`, not a validation error.

**Reasoning**: Most functions in the wild have no declared contracts. Most pipeline nodes in existing systems have no guarantees. Requiring a contract would make the schema unusable for the majority of real-world systems. The absence of a contract is itself useful information — it tells the agent where gaps exist and where to focus.

**Consequence**: The agent is the mechanism for filling gaps. A node with no contract and many execution traces is a candidate for automated contract proposal. The system captures what exists and surfaces what is missing. It does not block progress on what is absent.

---

### D12: Tests are a source of guarantees

**Decision**: Existing tests — unit tests, dbt tests, Great Expectations suites — are treated as a source of guarantees via `GuaranteeOrigin`. A Go test that asserts a precondition is a guarantee with `OriginExtractedTest`. A dbt `not_null` test is a guarantee with `OriginDbtTest`.

**Reasoning**: Developers have already expressed many guarantees in test code. Rather than asking them to write contracts from scratch, the system extracts what already exists. This lowers the adoption cost significantly — a codebase with good test coverage immediately has a rich guarantee layer.

**Current scope**: Capture and represent only. The system records where guarantees came from. Writing new tests from contracts is a future capability, not a current requirement.

**Consequence**: `GuaranteeOrigin` is required on every guarantee. The origin kind distinguishes developer-declared guarantees from test-extracted ones from discovered ones, which matters for trust weighting and audit trails.

---

### D13: Contract/Guarantee framing

**Decision**: The schema is framed as: contract = the agreement, guarantee = the test of that agreement. This framing is explicit in the schema document and drives the naming of all related types.

**Reasoning**: This framing clarifies the relationship between static declarations and runtime evidence. A contract without guarantees is just a shape declaration — useful, but incomplete. A guarantee without a contract is meaningless. An execution trace is the test run. A violation is the test failure. The framing maps directly onto how developers and data engineers already think about testing and quality.

**Consequence**: The feedback loop is the core of the system's value. Every trace tests every guarantee. Confidence accumulates. The system gets smarter about what is actually true versus what was merely declared.

---

### D14: Bitemporal valid time on contracts and guarantees

**Decision**: Contracts and guarantees use valid time bitemporal modeling — `valid_from` and `valid_until` on every version. Every `ExecutionTrace` stores a `contract_version_id` pointing to the exact contract that was active when it ran.

**Reasoning**: The core question that motivated this — "what did this pipeline look like when it ran at 2am on March 15th?" — cannot be answered without it. Storing only the current contract means history is lost. Storing only diffs means reconstruction is required. Valid time versioning stores the complete history directly and makes point-in-time lookup a simple range query.

**Research basis**: This is a well-established standard. SQL:2011 formalised bitemporal tables. PostgreSQL 18 adds native application-time temporal constraints. The pattern is used in financial systems, legal systems, and any domain where "what did we believe at time T?" is a meaningful question. Contracts are exactly this kind of data.

**Two time dimensions**: Valid time (when the contract was in effect) is implemented now. Transaction time (when the database recorded it — relevant for retroactive corrections) is deferred. The schema supports adding it later without rework.

**Blast radius connection**: Contract versioning closes the blast radius loop. A contract change has a `valid_from` timestamp. Edges have an `updated_at` timestamp. The agent can identify edges that haven't been rechecked since the last contract change without any additional schema. The agent handles the propagation logic — the schema provides the data.

**No database decisions made**: The schema expresses structure in standard SQL for clarity. The actual storage engine for each of the three databases (graph, observations, analysis) is an open implementation decision.

---

## Open Questions

### Q1: Storage layer

**Question**: The current GoraphDB storage layer shows seams at Phase 1 scale — O(N) scans for entry points and untested functions, free-form props map for schema. As the unified model adds richer node types, guarantee evidence arrays, and execution traces, GoraphDB becomes increasingly awkward. What replaces it?

**Options considered**:
- Keep GoraphDB for Phase 1, migrate at Phase 2
- Move to SQLite with explicit schema tables (good for typed queries, less natural for graph traversal)
- Move to a purpose-built embedded graph database (DGraph, Badger-based)
- Keep a graph store for traversal, add a relational store for traces and guarantees

**Why unresolved**: The right answer depends on the scale and query patterns of Phase 2. The schema is designed to be storage-agnostic — the types are defined independently of how they are persisted. The storage decision can be deferred until Phase 2 without affecting schema design.

**Constraint**: Whatever storage is chosen must support the agent's need to traverse in unexpected ways. Arbitrary graph traversal is the core capability. SQL joins for multi-hop traversal get unwieldy quickly. This argues against a pure relational approach.

---

### Q2: Schema versioning and evolution — RESOLVED

**Resolution**: Bitemporal valid time on contracts and guarantees (D14). Each contract and guarantee row has `valid_from` and `valid_until`. The `ExecutionTrace.contract_version_id` pins the exact version active at execution time. Point-in-time lookup is a simple range query. The diagnostic query "did violations start before or after the last contract change?" is directly answerable by comparing `MIN(violation.occurred_at)` against `MAX(contract.valid_from)` for a node.

---

### Q3: Environment boundaries and cross-environment edges

**Question**: When a Go function writes to a Kafka topic, that edge crosses an environment boundary — from the code environment to the infrastructure environment. How is this edge modeled, and what is the contract at the boundary?

**Partially answered**: The edge `Kind: "writes"` connects a function node to a stream node. The `DataShape` on the edge carries what flows. But the question of *format enforcement* at the boundary — whether Arrow IPC is enforced, whether schema validation happens at the edge — is not specified.

**Connector research relevance**: Your connector research uses Arrow IPC as the enforced wire format at environment boundaries. This should map to `DataShape.Format: "arrow"` on cross-environment edges. But the validation mechanism — who checks that what the function writes matches the Arrow schema — needs to be specified.

---

### Q4: Guarantee confidence thresholds

**Question**: What confidence level promotes a discovered guarantee from `experimental` to `locked`? What level triggers a transition from `locked` to `broken`?

**Why unresolved**: This is explicitly a policy decision, not a technical one — just as your connector research document states: "The threshold is a policy decision, not a technical one." The schema supports it (confidence and observation counts are tracked) but the thresholds themselves are not defined.

**Suggestion**: These should be configurable per environment, not hardcoded. A production environment might require 10,000 observations and 99.9% confidence to lock a guarantee. A development environment might require 100 observations and 95% confidence.

---

### Q5: Agent traversal API

**Question**: The MCP server currently exposes 13 tools covering the CodeValent query surface. What new tools does the unified model require, and how does the agent traverse across environment boundaries?

**Known additions needed**:
- `contract <node>` — get the full contract including guarantees
- `violations <node>` — get recent contract violations
- `breaking_edges` — all edges with `Compatibility: "breaking"`
- `trace_history <node>` — recent execution traces
- `guarantee_confidence <node>` — confidence levels for all guarantees
- `blast_radius_cross_environment <node>` — impact including storage and pipeline nodes

**Why unresolved**: The full tool surface depends on what agents actually need to do. The current 13 tools were designed for code-only queries. The unified model enables cross-system queries that don't exist yet. The tool surface should be designed after at least one end-to-end agent task is attempted against the unified model.

---

### Q6: PII tracking mechanism

**Question**: `GuaranteePII` marks a field as containing PII. But the mechanism for *tracing* PII flow through the graph — from the field where it enters to every node that handles it — is not specified.

**Why it matters**: Phase 4 in the roadmap includes PII path tracking. The guarantee kind is the right foundation. But the traversal query that answers "every node that handles this PII field" requires following edges where the field appears in the DataShape, which requires field-level edge tracking rather than just node-level contract tracking.

**Dependency**: This requires field-level lineage on edges — tracking not just that data flows from A to B, but that *this specific field* flows from A to B. The current schema tracks field shapes on nodes but not field-level provenance on edges.

---

### Q7: Multi-environment graph management

**Question**: The graph currently lives in a single `.cvalent/graph.db` file local to a project. In the unified model, a single system might span multiple environments — a codebase, an AWS account, a GCP project. How are these graphs related? Is there one graph per environment, federated at query time? Or one unified graph that spans all environments?

**Why it matters**: This is the Phase 2 cloud sync question. Local-first works for a single codebase. Cross-environment traversal requires either a central graph or a federation mechanism.

**Constraint**: Whatever the answer, the agent must be able to traverse from a function node in one environment to a storage node in another environment in a single query. That is the whole point of the unified model.

---

### Q8: Connector library as graph nodes

**Question**: Your connector research produces a library of locked patterns. Should these library entries be `KindConnector` nodes in the graph? If so, what is their contract — the schema they guarantee to produce or consume?

**Why it matters**: If connectors are nodes, the agent can answer "which nodes use this connector?" and "which connector is most reliable for this storage system?" If they are just configuration referenced by `PipelineMeta.ConnectorID`, they are invisible to the agent.

**Suggestion**: Connectors should be nodes. Their contract is the Arrow schema they guarantee to emit (for sources) or accept (for destinations). Their state reflects the connector library state: unknown, experimental, locked, broken. This makes the connector research directly visible in the graph.

---

## What Is Not In Scope

These are explicitly deferred and should not influence Phase 1 implementation:

- **Pricing and billing** — not a graph concern
- **User authentication and RBAC** — infrastructure, not schema
- **Dashboard and BI tool nodes** — downstream consumers, added when needed
- **ML feature store specifics** — `KindMLModel` is reserved but not detailed
- **Multi-tenancy** — single-tenant model for now, `EnvironmentID` provides future isolation point
- **Real-time streaming of graph updates** — batch rebuild is sufficient for Phase 1 and 2
