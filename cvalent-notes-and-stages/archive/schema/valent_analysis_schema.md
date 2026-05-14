# Valent Analysis Database Schema
**Version**: 0.2  
**Status**: Draft  
**Date**: April 2026

---

## Database Technology

Not yet decided. The schema below is expressed as standard SQL for clarity, but the actual storage engine is an open implementation decision. Constraints:

- Relatively low write volume compared to observations
- Mostly relational queries — proposals, reviews, status updates
- Needs to be queryable by agents via MCP so agents can read prior analyses before producing new ones
- PostgreSQL is a natural fit but nothing is locked

---

## Purpose

The analysis database holds agent-generated intelligence. It is the agent's working output — derived understanding produced by reading both the graph and observations. It contains shifts, anomalies, patterns, proposals, diagnoses, and recommendations.

The analysis database answers intelligence questions:
- What contract gaps should be addressed first?
- What shifts in the graph have occurred since last week?
- What does the agent recommend for this node's missing contract?
- What caused this incident?
- Which guarantees are candidates for promotion from experimental to locked?

The analysis database does not replace human judgment. It surfaces what the agent has determined so a human can review and decide. In the current phase, humans decide what gets promoted back into the graph. Eventually, high-confidence analyses with sufficient track record can be applied automatically.

---

## Design Principles

**Agent-written, human-reviewed**: Only agents write to this database. Humans read it and decide what to act on. The trust boundary is explicit.

**Versioned proposals**: Proposals are never overwritten. A new analysis produces a new record. The history of what the agent proposed and what was accepted or rejected is preserved.

**Linked to evidence**: Every analysis record links back to the graph entities and observation records that informed it. An agent diagnosis is only as trustworthy as the evidence it cites.

**Status lifecycle**: Every record has a status that tracks the human review cycle: `pending` → `accepted` / `rejected` / `superseded`.

---

## Common Fields

Every analysis record shares this envelope:

```sql
-- Every analysis table includes these fields
id              TEXT PRIMARY KEY
agent_id        TEXT NOT NULL      -- which agent produced this
agent_version   TEXT NOT NULL      -- version of the agent/harness
produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
status          TEXT NOT NULL DEFAULT 'pending'
                -- 'pending', 'accepted', 'rejected', 'superseded', 'applied'
reviewed_by     TEXT               -- who reviewed it
reviewed_at     TIMESTAMP
review_notes    TEXT
confidence      REAL NOT NULL      -- agent's self-assessed confidence 0.0-1.0
evidence        JSONB              -- references to graph/observation records used
```

---

## Tables

### `shift_detections`

Records when the agent detects a meaningful change in the system — a contract drift, a guarantee degrading, a new pattern emerging, a node that was stable starting to fail.

```sql
CREATE TABLE shift_detections (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- What shifted
    shift_kind      TEXT NOT NULL,      -- ShiftKind
    entity_kind     TEXT NOT NULL,      -- 'node', 'edge', 'guarantee', 'contract'
    entity_id       TEXT NOT NULL,      -- ID in the graph

    -- The shift
    summary         TEXT NOT NULL,      -- human-readable one-line summary
    detail          TEXT,               -- full explanation
    previous_state  JSONB,              -- snapshot of state before
    current_state   JSONB,              -- snapshot of current state
    severity        TEXT NOT NULL,      -- 'critical', 'warning', 'info'

    -- Evidence
    evidence        JSONB NOT NULL,     -- trace IDs, observation IDs that support this

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT
);

CREATE INDEX idx_shifts_entity     ON shift_detections(entity_id);
CREATE INDEX idx_shifts_kind       ON shift_detections(shift_kind);
CREATE INDEX idx_shifts_severity   ON shift_detections(severity);
CREATE INDEX idx_shifts_status     ON shift_detections(status);
CREATE INDEX idx_shifts_produced   ON shift_detections(produced_at DESC);
```

```go
type ShiftKind string

const (
    ShiftConfidenceDegrading    ShiftKind = "confidence_degrading"     // guarantee confidence falling
    ShiftNewViolationPattern    ShiftKind = "new_violation_pattern"    // violations started after a change
    ShiftContractDrift          ShiftKind = "contract_drift"           // observed shape diverging from declared
    ShiftEdgeBreaking           ShiftKind = "edge_breaking"            // edge became incompatible
    ShiftNodeUnstable           ShiftKind = "node_unstable"            // node failure rate increasing
    ShiftUnexpectedDataShape    ShiftKind = "unexpected_data_shape"    // new field appearing in traces
    ShiftGuaranteeCandidate     ShiftKind = "guarantee_candidate"      // pattern strong enough to propose
    ShiftCoverageDecreasing     ShiftKind = "coverage_decreasing"      // governance coverage declining
)
```

---

### `anomaly_reports`

Records specific anomalies the agent detected in observation data — unusual patterns, outliers, correlations between failures.

```sql
CREATE TABLE anomaly_reports (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- What was anomalous
    anomaly_kind    TEXT NOT NULL,      -- AnomalyKind
    node_id         TEXT,               -- primary node involved
    related_nodes   JSONB,              -- other nodes involved

    -- The anomaly
    summary         TEXT NOT NULL,
    detail          TEXT,
    affected_period TSRANGE NOT NULL,   -- time window of the anomaly
    baseline        JSONB,              -- what normal looks like
    observed        JSONB,              -- what was actually seen

    -- Evidence
    evidence        JSONB NOT NULL,

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT
);

CREATE INDEX idx_anomalies_node     ON anomaly_reports(node_id);
CREATE INDEX idx_anomalies_kind     ON anomaly_reports(anomaly_kind);
CREATE INDEX idx_anomalies_status   ON anomaly_reports(status);
CREATE INDEX idx_anomalies_produced ON anomaly_reports(produced_at DESC);
```

```go
type AnomalyKind string

const (
    AnomalyViolationSpike         AnomalyKind = "violation_spike"          // sudden increase in violations
    AnomalyLatencyIncrease        AnomalyKind = "latency_increase"         // execution time growing
    AnomalyUnexpectedNulls        AnomalyKind = "unexpected_nulls"         // nulls appearing in non-null fields
    AnomalyCorrelatedFailures     AnomalyKind = "correlated_failures"      // multiple nodes failing together
    AnomalySchemaEvolution        AnomalyKind = "schema_evolution"         // shape changing over time
    AnomalyGuaranteeDivergence    AnomalyKind = "guarantee_divergence"     // declared vs observed diverging
)
```

---

### `contract_proposals`

Agent-proposed contracts for nodes that currently have none or have incomplete contracts. The human reviews and decides whether to accept.

```sql
CREATE TABLE contract_proposals (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- Target
    node_id         TEXT NOT NULL,
    proposal_kind   TEXT NOT NULL,   -- 'new_contract', 'extend_contract', 'correct_contract'

    -- The proposal
    proposed_inputs     JSONB,    -- [{name, type, nullable, expanded}]
    proposed_outputs    JSONB,    -- [{name, type, nullable, expanded}]
    completeness        TEXT NOT NULL DEFAULT 'inferred',
    reasoning           TEXT,     -- why the agent proposes this

    -- Evidence base
    trace_count         INTEGER,  -- how many traces this is based on
    observation_window  TSRANGE,  -- time period of observations used
    evidence            JSONB NOT NULL,

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT,

    -- If accepted, which contract was created/updated
    applied_contract_id TEXT
);

CREATE INDEX idx_proposals_node     ON contract_proposals(node_id);
CREATE INDEX idx_proposals_status   ON contract_proposals(status);
CREATE INDEX idx_proposals_produced ON contract_proposals(produced_at DESC);
```

---

### `guarantee_proposals`

Agent-proposed guarantees for nodes that have contracts but missing or incomplete guarantees. Sourced from patterns in execution traces, existing tests, or inference from similar nodes.

```sql
CREATE TABLE guarantee_proposals (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- Target
    node_id         TEXT NOT NULL,
    contract_id     TEXT NOT NULL,
    proposal_kind   TEXT NOT NULL,   -- 'new_guarantee', 'promote_discovered', 'correct_guarantee'

    -- The proposal
    when_applies    TEXT NOT NULL,   -- GuaranteeWhen
    category        TEXT NOT NULL,   -- GuaranteeCategory
    kind            TEXT NOT NULL,   -- GuaranteeKind
    field_name      TEXT,
    params          JSONB,
    origin_kind     TEXT NOT NULL,   -- GuaranteeOriginKind
    origin_ref      TEXT,
    reasoning       TEXT,

    -- Evidence base
    trace_count         INTEGER,
    observed_confidence REAL,        -- what confidence the agent observed
    observation_window  TSRANGE,
    evidence            JSONB NOT NULL,

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT,

    -- If accepted, which guarantee was created
    applied_guarantee_id TEXT
);

CREATE INDEX idx_guar_proposals_node     ON guarantee_proposals(node_id);
CREATE INDEX idx_guar_proposals_status   ON guarantee_proposals(status);
CREATE INDEX idx_guar_proposals_produced ON guarantee_proposals(produced_at DESC);
```

---

### `incident_diagnoses`

When something breaks, the agent reads traces, violations, state changes, and confidence history to produce a diagnosis. This records that diagnosis — the causal chain the agent constructed, the root cause it identified, and the remediation it recommends.

```sql
CREATE TABLE incident_diagnoses (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- What broke
    trigger_node_id     TEXT NOT NULL,   -- the node that first showed the failure
    trigger_trace_id    TEXT,            -- the trace that triggered diagnosis
    affected_nodes      JSONB,           -- all nodes in the blast radius

    -- The diagnosis
    summary             TEXT NOT NULL,
    root_cause          TEXT,            -- agent's best determination of root cause
    causal_chain        JSONB,           -- ordered sequence of events leading to failure
    contributing_factors JSONB,          -- other factors that made it worse

    -- Remediation
    recommended_actions JSONB,           -- [{action, priority, node_id, reasoning}]

    -- Evidence
    traces_examined     INTEGER,
    evidence            JSONB NOT NULL,

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT
);

CREATE INDEX idx_diagnoses_node     ON incident_diagnoses(trigger_node_id);
CREATE INDEX idx_diagnoses_status   ON incident_diagnoses(status);
CREATE INDEX idx_diagnoses_produced ON incident_diagnoses(produced_at DESC);
```

---

### `impact_assessments`

When a contract change is proposed or a node is about to be modified, the agent produces an impact assessment — what will break, what will degrade, what is safe to change.

```sql
CREATE TABLE impact_assessments (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- What is changing
    subject_node_id     TEXT NOT NULL,
    proposed_change     JSONB NOT NULL,  -- description of the proposed change

    -- Assessment
    breaking_edges      JSONB,   -- edges that will become incompatible
    affected_nodes      JSONB,   -- nodes downstream that will be affected
    safe_to_proceed     BOOLEAN,
    risk_summary        TEXT,
    detail              TEXT,

    -- Evidence
    evidence            JSONB NOT NULL,

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT
);

CREATE INDEX idx_assessments_node     ON impact_assessments(subject_node_id);
CREATE INDEX idx_assessments_status   ON impact_assessments(status);
CREATE INDEX idx_assessments_produced ON impact_assessments(produced_at DESC);
```

---

### `trend_analyses`

Periodic agent-generated summaries of system health trends. Not tied to a specific incident or change — a regular pulse check on the system's governance health.

```sql
CREATE TABLE trend_analyses (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    agent_version   TEXT NOT NULL,

    -- Scope
    scope_kind      TEXT NOT NULL,   -- 'environment', 'node', 'domain', 'system'
    scope_id        TEXT,            -- environment_id or node_id
    analysis_period TSRANGE NOT NULL,

    -- Trends
    summary         TEXT NOT NULL,
    contract_coverage_delta  REAL,   -- % change in nodes with contracts
    guarantee_coverage_delta REAL,   -- % change in nodes with guarantees
    violation_rate_delta     REAL,   -- % change in violation rate
    confidence_delta         REAL,   -- average confidence change across guarantees
    new_gaps                 INTEGER,
    resolved_gaps            INTEGER,
    detail                   JSONB,  -- full breakdown

    -- Lifecycle
    status          TEXT NOT NULL DEFAULT 'pending',
    confidence      REAL NOT NULL,
    produced_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by     TEXT,
    reviewed_at     TIMESTAMP,
    review_notes    TEXT
);

CREATE INDEX idx_trends_scope    ON trend_analyses(scope_id);
CREATE INDEX idx_trends_produced ON trend_analyses(produced_at DESC);
```

---

## Status Lifecycle

Every record in the analysis database follows the same lifecycle:

```
pending     →  produced by agent, not yet reviewed
accepted    →  human reviewed and approved
rejected    →  human reviewed and declined (with notes)
superseded  →  a newer analysis replaced this one
applied     →  accepted and the change has been made to the graph
```

For the current phase, `applied` requires human action. In future phases, `accepted` can automatically become `applied` for high-confidence analyses from trusted agents.

---

## Relationship to the Graph

When a proposal is accepted and applied, the graph is updated:

| Analysis Record | Graph Update |
|---|---|
| `contract_proposals` accepted | New `contracts` row + `contract_fields` rows |
| `guarantee_proposals` accepted | New `guarantees` row |
| `shift_detections` accepted | `nodes.state` or `edges.compatibility` updated |
| `incident_diagnoses` accepted | May trigger contract corrections or state changes |

The `applied_contract_id` and `applied_guarantee_id` fields on proposals record what was created in the graph as a result. This closes the audit trail — you can always trace a graph entity back to the analysis that proposed it.

---

## Notes

- The analysis database is the foundation for eventual automatic graph updates. When agent confidence is consistently high and human acceptance rate approaches 100% for a given analysis type, the review step can be removed and `pending` transitions directly to `applied`.
- `evidence` JSONB fields should contain structured references: `{"trace_ids": [...], "observation_ids": [...], "node_ids": [...]}`. This allows the UI to link directly to the source evidence.
- Agent versioning (`agent_id`, `agent_version`) enables evaluation of different agent configurations against each other — the equivalent of Meta-Harness running multiple harness candidates and comparing results.
- The analysis database should be queryable by the MCP server so agents can read prior analyses before producing new ones. This prevents redundant analyses and allows the agent to build on prior work.
