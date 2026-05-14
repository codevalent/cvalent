# Valent Observations Database Schema
**Version**: 0.2  
**Status**: Draft  
**Date**: April 2026

---

## Purpose

The observations database holds runtime events. It is the logbook of the system — what actually happened, when, and what the outcome was. It is append-only and time-series oriented. Nothing is deleted or updated. New events are always written as new rows.

The observations database answers temporal questions:
- How has confidence on this guarantee changed over the last 30 days?
- What was the violation rate last week vs this week?
- When did this node first start breaking?
- Show me all traces where amount was null
- What contract was in effect when this trace ran?
- Did violations start before or after the last contract change?

The observations database does not hold structure. It references nodes, edges, and contracts by ID but does not duplicate their definitions. Structure and contract history live in the Graph database.

---

## Database Technology

Not yet decided. The schema below is expressed as standard SQL for clarity, but the actual storage engine is an open implementation decision. Constraints:

- Must be optimized for append-only writes at high volume
- Must support efficient time-range queries
- Must handle large text fields (logs, stack traces) gracefully
- Options include: TimescaleDB, ClickHouse, PostgreSQL with partitioning, managed time-series stores

---

## Design Principles

**Append-only**: No updates, no deletes. Every event is immutable. Corrections are new events.

**Time-series first**: Every table has a timestamp. Queries are almost always time-bounded. Indexes are built for time-range scans.

**Reference by ID**: All references to graph entities use IDs. The observations store does not duplicate graph structure — it references it.

**Contract pinning**: Every `ExecutionTrace` stores a direct reference to the exact contract version that was active when it ran. This closes the temporal resolution loop — no reconstruction needed. Join to the graph to get the full contract at any time.

---

## Tables

### `execution_traces`

One row per node execution. Written by the runtime — pipeline runs, OpenLineage events, test executions, agent-observed runs.

The `contract_version_id` field is the answer to "what did this pipeline look like when it ran?" — it points directly to the contract row in the graph that was active at `started_at`. No reconstruction required.

```sql
CREATE TABLE execution_traces (
    -- Identity
    id              TEXT NOT NULL,
    node_id         TEXT NOT NULL,    -- references graph nodes(id)
    run_id          TEXT,             -- OpenLineage run ID if from external tool

    -- Contract pinning — exact contract active when this trace ran
    -- References graph contracts(id). Resolved at trace write time.
    contract_version_id TEXT,         -- null if node had no contract when it ran

    -- Timing
    started_at      TIMESTAMPTZ NOT NULL,
    completed_at    TIMESTAMPTZ,
    duration_ms     INTEGER,

    -- Outcome
    status          TEXT NOT NULL,    -- 'success', 'failure', 'partial', 'timeout'

    -- Source
    source          TEXT NOT NULL,    -- 'openlineage', 'native', 'test', 'agent'

    -- Observed shapes — what actually flowed, not what was declared
    input_shape     JSONB,            -- {fields: [{name, type, nullable}], format, completeness}
    output_shape    JSONB,

    -- Context for agent diagnosis
    logs            TEXT[],
    error_msg       TEXT,
    stack_trace     TEXT,
    engine_version  TEXT,
    connector_state TEXT,
    external_run_id TEXT,             -- Airflow run_id, Spark job_id, dbt run_id
    external_url    TEXT,             -- link back to source system

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id, started_at)      -- partition key included for time-series stores
)
PARTITION BY RANGE (started_at);      -- partition by month in production

CREATE INDEX idx_traces_node_id    ON execution_traces(node_id);
CREATE INDEX idx_traces_started_at ON execution_traces(started_at DESC);
CREATE INDEX idx_traces_status     ON execution_traces(status);
CREATE INDEX idx_traces_source     ON execution_traces(source);
CREATE INDEX idx_traces_contract   ON execution_traces(contract_version_id);
-- Most common query: recent traces for a node
CREATE INDEX idx_traces_node_time  ON execution_traces(node_id, started_at DESC);
```

**Resolving contract_version_id at write time:**
```sql
-- When writing a trace, look up the active contract for the node at started_at
SELECT id FROM contracts
WHERE node_id = :node_id
  AND valid_from <= :started_at
  AND (valid_until IS NULL OR valid_until > :started_at)
LIMIT 1;
```

**Full contract reconstruction for a trace:**
```sql
-- What did the contract look like when this trace ran?
SELECT t.id as trace_id, t.started_at, t.status,
       c.version, c.completeness,
       cf.direction, cf.name, cf.type, cf.nullable
FROM execution_traces t
JOIN contracts c ON c.id = t.contract_version_id
JOIN contract_fields cf ON cf.contract_id = c.id
WHERE t.id = :trace_id;
```

---

### `contract_violations`

One row per guarantee failure within an execution. A single trace can produce multiple violations. Each violation references both the trace and the guarantee that failed.

```sql
CREATE TABLE contract_violations (
    -- Identity
    id              TEXT NOT NULL,
    trace_id        TEXT NOT NULL,
    node_id         TEXT NOT NULL,    -- denormalized for query performance
    guarantee_id    TEXT NOT NULL,    -- references graph guarantees(id)

    -- What failed
    guarantee_kind  TEXT NOT NULL,    -- GuaranteeKind — denormalized
    when_applies    TEXT NOT NULL,    -- 'precondition', 'postcondition', 'invariant'
    field_name      TEXT,

    -- Evidence
    expected        TEXT,
    observed        TEXT,
    sample_value    TEXT,
    severity        TEXT NOT NULL,    -- 'breaking', 'warning', 'info'

    occurred_at     TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (id, occurred_at)
)
PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_violations_trace     ON contract_violations(trace_id);
CREATE INDEX idx_violations_guarantee ON contract_violations(guarantee_id);
CREATE INDEX idx_violations_node      ON contract_violations(node_id);
CREATE INDEX idx_violations_kind      ON contract_violations(guarantee_kind);
CREATE INDEX idx_violations_time      ON contract_violations(occurred_at DESC);
CREATE INDEX idx_violations_severity  ON contract_violations(severity);
CREATE INDEX idx_violations_node_time ON contract_violations(node_id, occurred_at DESC);
```

---

### `node_state_changes`

Records every transition in node state. Current state lives in the graph. History lives here.

```sql
CREATE TABLE node_state_changes (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL,
    previous_state  TEXT NOT NULL,    -- NodeState
    current_state   TEXT NOT NULL,    -- NodeState
    reason          TEXT,
    triggered_by    TEXT,             -- 'feedback_loop', 'agent', 'human', 'system'
    trace_id        TEXT,             -- which trace triggered the change, if any
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_state_node      ON node_state_changes(node_id);
CREATE INDEX idx_state_time      ON node_state_changes(occurred_at DESC);
CREATE INDEX idx_state_node_time ON node_state_changes(node_id, occurred_at DESC);
```

---

### `guarantee_confidence_history`

Records every update to a guarantee's confidence score. Current score lives in the graph. Time series lives here.

This answers: "was confidence trending down before the incident?"

```sql
CREATE TABLE guarantee_confidence_history (
    id              TEXT NOT NULL,
    guarantee_id    TEXT NOT NULL,    -- references graph guarantees(id)
    node_id         TEXT NOT NULL,    -- denormalized
    guarantee_kind  TEXT NOT NULL,    -- denormalized

    -- Snapshot at this point in time
    confidence      REAL NOT NULL,
    observations    INTEGER NOT NULL,
    violations      INTEGER NOT NULL,

    -- What triggered this update
    triggered_by_trace_id TEXT,

    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id, recorded_at)
)
PARTITION BY RANGE (recorded_at);

CREATE INDEX idx_confidence_guarantee  ON guarantee_confidence_history(guarantee_id);
CREATE INDEX idx_confidence_node       ON guarantee_confidence_history(node_id);
CREATE INDEX idx_confidence_time       ON guarantee_confidence_history(recorded_at DESC);
CREATE INDEX idx_confidence_node_time  ON guarantee_confidence_history(node_id, recorded_at DESC);
```

---

### `edge_compatibility_changes`

Records every time an edge's compatibility status changed. Current status lives in the graph. History lives here.

```sql
CREATE TABLE edge_compatibility_changes (
    id              TEXT PRIMARY KEY,
    edge_id         TEXT NOT NULL,
    from_node_id    TEXT NOT NULL,    -- denormalized
    to_node_id      TEXT NOT NULL,    -- denormalized
    previous_compat TEXT NOT NULL,    -- EdgeCompatibility
    current_compat  TEXT NOT NULL,    -- EdgeCompatibility
    reason          TEXT,
    triggered_by    TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_compat_edge ON edge_compatibility_changes(edge_id);
CREATE INDEX idx_compat_time ON edge_compatibility_changes(occurred_at DESC);
```

---

### `gap_detection_events`

Records when contract gaps were detected or resolved. Enables trending: are we gaining or losing governance coverage over time?

```sql
CREATE TABLE gap_detection_events (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL,
    gap_kind    TEXT NOT NULL,   -- ContractGapKind
    field_name  TEXT,
    event_type  TEXT NOT NULL,   -- 'detected' or 'resolved'
    resolved_by TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_gaps_node ON gap_detection_events(node_id);
CREATE INDEX idx_gaps_time ON gap_detection_events(occurred_at DESC);
CREATE INDEX idx_gaps_kind ON gap_detection_events(gap_kind);
```

---

## Feedback Loop Queries

These queries are run by the feedback loop to update confidence scores in the graph after new traces arrive.

### Confidence update for a guarantee
```sql
-- Violation rate for a guarantee over the last 30 days
SELECT
    COUNT(*)                                              AS total_traces,
    COUNT(cv.id)                                          AS violated_traces,
    1.0 - (COUNT(cv.id)::REAL / NULLIF(COUNT(*), 0))     AS current_confidence
FROM execution_traces et
LEFT JOIN contract_violations cv
    ON cv.trace_id = et.id
    AND cv.guarantee_id = :guarantee_id
WHERE et.node_id   = :node_id
  AND et.started_at > NOW() - INTERVAL '30 days';
```

### Did violations start before or after the last contract change?
```sql
-- First violation for this node
SELECT MIN(cv.occurred_at) AS first_violation
FROM contract_violations cv
WHERE cv.node_id = :node_id
  AND cv.guarantee_kind = :guarantee_kind;

-- Last contract change for this node (query the graph)
-- SELECT MAX(valid_from) FROM contracts WHERE node_id = :node_id;

-- If first_violation > last contract change: the new contract introduced the problem
-- If first_violation < last contract change: the problem predates the change
```

### Recent violation rate for a node
```sql
SELECT
    guarantee_kind,
    COUNT(*)           AS violation_count,
    MIN(occurred_at)   AS first_seen,
    MAX(occurred_at)   AS last_seen
FROM contract_violations
WHERE node_id     = :node_id
  AND occurred_at > NOW() - INTERVAL '7 days'
GROUP BY guarantee_kind
ORDER BY violation_count DESC;
```

### Confidence trend for agent diagnosis
```sql
SELECT recorded_at, confidence, observations, violations
FROM guarantee_confidence_history
WHERE guarantee_id = :guarantee_id
ORDER BY recorded_at DESC
LIMIT 100;
```

---

## Retention

| Table | Suggested Retention |
|---|---|
| `execution_traces` | 90 days full, 1 year aggregated |
| `contract_violations` | 1 year |
| `node_state_changes` | Permanent (low volume) |
| `guarantee_confidence_history` | 1 year |
| `edge_compatibility_changes` | Permanent (low volume) |
| `gap_detection_events` | 1 year |

High-volume tables (`execution_traces`, `contract_violations`, `guarantee_confidence_history`) should be partitioned by time.

---

## Notes

- `contract_version_id` on `execution_traces` is the key addition in v0.2. It is resolved at trace write time by looking up the active contract for the node at `started_at`. This means every trace permanently knows what contract it ran against — no reconstruction needed.
- All `node_id`, `guarantee_id`, `edge_id` fields reference the Graph database. These are not foreign keys — referential integrity is enforced at the application layer since these are separate databases.
- JSONB columns (`input_shape`, `output_shape`) store variable-structure observed data. The graph has the canonical declared schema; observations capture what actually flowed.
- `execution_traces.logs` is a text array. In high-volume deployments this should move to object storage (S3/GCS) with only a reference stored here.
- All timestamps use `TIMESTAMPTZ`. Pipelines span timezones. Naive timestamps cause subtle bugs.
