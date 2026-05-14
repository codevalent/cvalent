# Valent Graph Database Schema
**Version**: 0.2  
**Status**: Draft  
**Date**: April 2026

---

## Purpose

The graph database holds structure and contracts. It is the map of the system — what exists, what it promises, how it connects. It is built by humans with agent assistance and changes when code changes, pipelines are reconfigured, or contracts are declared.

The graph answers structural questions:
- What calls this function?
- What is the blast radius of changing this node?
- Which edges are breaking?
- What nodes have no contract?
- How does data flow from this source to that destination?
- What did this contract look like when this trace ran?

The graph does not hold execution history. It holds current state and the full history of how contracts and guarantees have changed over time. Execution events live in the Observations database.

---

## Database Technology

Not yet decided. The schema below is expressed as standard SQL for clarity, but the actual storage engine is an open implementation decision. Constraints:

- Must support efficient graph traversal (agent needs to traverse in unexpected ways)
- Must support bitemporal queries on contracts and guarantees (valid_from/valid_until range lookups)
- Must handle the unified node model across code, pipeline, and infrastructure kinds

---

## Temporal Modeling

Contracts and guarantees use **valid time** bitemporal modeling. This answers the core question: *what did this contract look like at a specific point in time?*

Every contract and guarantee row has:
- `valid_from` — when this version became active in the real world
- `valid_until` — when it was superseded (null = currently active)

This means:
- There is never a single "current" contract row per node — there is the row where `valid_until IS NULL`
- Point-in-time lookup is a range query, not a join to a "current" table
- History is never deleted — old versions remain forever
- Each `ExecutionTrace` stores a `contract_version_id` — a direct reference to the exact contract that was active when it ran

**Transaction time** (when the database recorded the fact, as distinct from when it was true) is deferred. It becomes relevant if retroactive contract corrections are needed. The schema is designed to add it later without rework — `valid_from`/`valid_until` columns are already the right foundation.

---

## Tables

### `nodes`

```sql
CREATE TABLE nodes (
    -- Identity
    id              TEXT PRIMARY KEY,   -- deterministic UUID from (environment_id, kind, qualified_name)
    kind            TEXT NOT NULL,      -- NodeKind
    name            TEXT NOT NULL,
    qualified_name  TEXT NOT NULL,
    environment_id  TEXT NOT NULL,

    -- Operational
    state           TEXT NOT NULL DEFAULT 'unknown',  -- NodeState
    source          TEXT NOT NULL,                    -- NodeSource

    -- Trigger
    trigger_kind     TEXT,
    trigger_schedule TEXT,   -- cron expression
    trigger_event_id TEXT,   -- source node ID

    -- Engine
    engine_language TEXT,
    engine_runtime  TEXT,
    engine_version  TEXT,

    -- Timestamps
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (environment_id, kind, qualified_name)
);

CREATE INDEX idx_nodes_kind          ON nodes(kind);
CREATE INDEX idx_nodes_environment   ON nodes(environment_id);
CREATE INDEX idx_nodes_state         ON nodes(state);
CREATE INDEX idx_nodes_qualified     ON nodes(environment_id, kind, qualified_name);
```

---

### `node_function_meta`
*One row per node where kind = 'function'*

```sql
CREATE TABLE node_function_meta (
    node_id     TEXT PRIMARY KEY REFERENCES nodes(id),
    language    TEXT NOT NULL,
    package     TEXT,
    receiver    TEXT,
    is_method   BOOLEAN NOT NULL DEFAULT FALSE,
    start_line  INTEGER,
    end_line    INTEGER,
    exported    BOOLEAN NOT NULL DEFAULT FALSE,
    tag         TEXT NOT NULL DEFAULT 'application'  -- 'application' or 'test'
);
```

---

### `node_pipeline_meta`
*One row per node where kind = 'pipeline_step'*

```sql
CREATE TABLE node_pipeline_meta (
    node_id             TEXT PRIMARY KEY REFERENCES nodes(id),
    role                TEXT NOT NULL,      -- 'source', 'compute', 'destination'
    connector_id        TEXT,
    connector_state     TEXT NOT NULL DEFAULT 'unknown',
    credential_provider TEXT,               -- reference only, never the value
    credential_key      TEXT,               -- reference only, never the value
    config              JSONB               -- connector configuration key/value pairs
);
```

---

### `node_storage_meta`
*One row per node where kind = 'table', 'stream', or 'bucket'*

```sql
CREATE TABLE node_storage_meta (
    node_id         TEXT PRIMARY KEY REFERENCES nodes(id),
    system          TEXT NOT NULL,   -- 'postgres', 'bigquery', 'kafka', 's3'
    database        TEXT,
    schema          TEXT,
    table_name      TEXT,
    format          TEXT,            -- 'parquet', 'avro', 'json', 'csv', 'delta'
    partition_key   TEXT,
    retention_days  INTEGER
);
```

---

### `node_endpoint_meta`
*One row per node where kind = 'endpoint'*

```sql
CREATE TABLE node_endpoint_meta (
    node_id     TEXT PRIMARY KEY REFERENCES nodes(id),
    protocol    TEXT NOT NULL,   -- 'http', 'grpc', 'graphql'
    method      TEXT,            -- 'GET', 'POST', ''
    path        TEXT,
    auth_scheme TEXT             -- 'bearer', 'api_key', 'oauth', 'none'
);
```

---

### `contracts`

Bitemporal. Each row represents a contract version with the period during which it was active. A node can have many contract versions over time. The currently active version is the row where `valid_until IS NULL`.

```sql
CREATE TABLE contracts (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL REFERENCES nodes(id),
    version         TEXT NOT NULL,      -- semantic version: "1", "2", "1.1"
    completeness    TEXT NOT NULL DEFAULT 'inferred',   -- 'full', 'partial', 'inferred'

    -- Valid time: when was this contract version in effect
    valid_from      TIMESTAMPTZ NOT NULL,
    valid_until     TIMESTAMPTZ,        -- null = currently active

    -- What changed from the previous version (null for first version)
    change_summary  TEXT,
    is_breaking     BOOLEAN NOT NULL DEFAULT FALSE,
    changed_by      TEXT,               -- 'parsed', 'openlineage', 'agent', 'human'

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- No two active contracts for the same node at the same time
CREATE UNIQUE INDEX idx_contracts_node_active
    ON contracts(node_id)
    WHERE valid_until IS NULL;

-- Point-in-time lookup: what contract was active at timestamp T?
CREATE INDEX idx_contracts_node_valid
    ON contracts(node_id, valid_from, valid_until);

CREATE INDEX idx_contracts_node_version
    ON contracts(node_id, version);
```

**Point-in-time query:**
```sql
-- What contract was active when a trace ran at :moment?
SELECT c.*
FROM contracts c
WHERE c.node_id = :node_id
  AND c.valid_from <= :moment
  AND (c.valid_until IS NULL OR c.valid_until > :moment);
```

**Superseding a contract** (creating a new version):
```sql
-- 1. Close the current version
UPDATE contracts
SET valid_until = NOW()
WHERE node_id = :node_id AND valid_until IS NULL;

-- 2. Insert the new version
INSERT INTO contracts (id, node_id, version, completeness, valid_from, change_summary, is_breaking, changed_by)
VALUES (:new_id, :node_id, :new_version, :completeness, NOW(), :summary, :is_breaking, :changed_by);
```

---

### `contract_fields`
*Inputs and outputs of a contract version*

```sql
CREATE TABLE contract_fields (
    id           TEXT PRIMARY KEY,
    contract_id  TEXT NOT NULL REFERENCES contracts(id),
    direction    TEXT NOT NULL,   -- 'input' or 'output'
    name         TEXT NOT NULL,
    type         TEXT,            -- null if unknown/inferred
    nullable     BOOLEAN NOT NULL DEFAULT FALSE,
    position     INTEGER NOT NULL DEFAULT 0,

    UNIQUE (contract_id, direction, name)
);

CREATE INDEX idx_fields_contract   ON contract_fields(contract_id);
CREATE INDEX idx_fields_direction  ON contract_fields(contract_id, direction);
```

---

### `contract_field_expansions`
*One-hop struct/object field expansion*

```sql
CREATE TABLE contract_field_expansions (
    id       TEXT PRIMARY KEY,
    field_id TEXT NOT NULL REFERENCES contract_fields(id),
    name     TEXT NOT NULL,
    type     TEXT,
    nullable BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_expansions_field ON contract_field_expansions(field_id);
```

---

### `guarantees`

Bitemporal. Each row represents a guarantee version with the period during which it was active. Guarantees can be added, removed, or changed over time. The currently active version is the row where `valid_until IS NULL`.

```sql
CREATE TABLE guarantees (
    id            TEXT PRIMARY KEY,
    contract_id   TEXT NOT NULL REFERENCES contracts(id),
    field_name    TEXT,           -- null means applies to whole node

    -- Classification
    when_applies  TEXT NOT NULL,  -- 'precondition', 'postcondition', 'invariant'
    category      TEXT NOT NULL,  -- GuaranteeCategory
    kind          TEXT NOT NULL,  -- GuaranteeKind

    -- Source and origin
    source        TEXT NOT NULL,  -- 'declared' or 'discovered'
    origin_kind   TEXT NOT NULL,  -- GuaranteeOriginKind
    origin_ref    TEXT,           -- file path, test name, suite name
    origin_line   INTEGER,

    -- Evidence — updated by feedback loop from observations
    confidence    REAL NOT NULL DEFAULT 1.0,
    observations  INTEGER NOT NULL DEFAULT 0,
    violations    INTEGER NOT NULL DEFAULT 0,

    -- Kind-specific parameters
    params        JSONB,

    -- Valid time: when was this guarantee version in effect
    valid_from    TIMESTAMPTZ NOT NULL,
    valid_until   TIMESTAMPTZ,    -- null = currently active

    -- What changed (null for first version)
    change_summary TEXT,
    changed_by    TEXT,           -- 'parsed', 'openlineage', 'agent', 'human'

    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- No two active guarantees of the same kind on the same field for the same contract
CREATE UNIQUE INDEX idx_guarantees_active
    ON guarantees(contract_id, kind, COALESCE(field_name, ''))
    WHERE valid_until IS NULL;

-- Point-in-time lookup
CREATE INDEX idx_guarantees_contract_valid
    ON guarantees(contract_id, valid_from, valid_until);

CREATE INDEX idx_guarantees_kind       ON guarantees(kind);
CREATE INDEX idx_guarantees_source     ON guarantees(source);
CREATE INDEX idx_guarantees_confidence ON guarantees(confidence);
```

---

### `edges`

```sql
CREATE TABLE edges (
    id             TEXT PRIMARY KEY,
    kind           TEXT NOT NULL,   -- EdgeKind
    from_id        TEXT NOT NULL REFERENCES nodes(id),
    to_id          TEXT NOT NULL REFERENCES nodes(id),

    -- Shape flowing across this edge
    data_shape_format       TEXT,
    data_shape_completeness TEXT NOT NULL DEFAULT 'inferred',

    -- Contract compatibility — updated when either endpoint contract changes
    compatibility  TEXT NOT NULL DEFAULT 'unknown',  -- EdgeCompatibility

    -- Execution history
    last_observed_at   TIMESTAMPTZ,
    observation_count  INTEGER NOT NULL DEFAULT 0,

    -- Source
    source         TEXT NOT NULL,   -- EdgeSource

    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (kind, from_id, to_id)
);

CREATE INDEX idx_edges_from          ON edges(from_id);
CREATE INDEX idx_edges_to            ON edges(to_id);
CREATE INDEX idx_edges_kind          ON edges(kind);
CREATE INDEX idx_edges_compatibility ON edges(compatibility);
```

---

### `edge_data_shape_fields`
*Fields flowing across an edge*

```sql
CREATE TABLE edge_data_shape_fields (
    id       TEXT PRIMARY KEY,
    edge_id  TEXT NOT NULL REFERENCES edges(id),
    name     TEXT NOT NULL,
    type     TEXT,
    nullable BOOLEAN NOT NULL DEFAULT FALSE,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_edge_fields_edge ON edge_data_shape_fields(edge_id);
```

---

### `environments`

```sql
CREATE TABLE environments (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,   -- 'codebase', 'aws_account', 'gcp_project', 'pipeline'
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## Computed Views

These are queries the system runs on demand. The primary interface for agents and CLI tools.

### Current contract for a node
```sql
SELECT c.*, cf.*
FROM contracts c
JOIN contract_fields cf ON cf.contract_id = c.id
WHERE c.node_id = :node_id
  AND c.valid_until IS NULL;
```

### Contract at a point in time
```sql
SELECT c.*, cf.*
FROM contracts c
JOIN contract_fields cf ON cf.contract_id = c.id
WHERE c.node_id = :node_id
  AND c.valid_from <= :moment
  AND (c.valid_until IS NULL OR c.valid_until > :moment);
```

### Contract history for a node
```sql
SELECT c.version, c.valid_from, c.valid_until,
       c.is_breaking, c.change_summary, c.changed_by
FROM contracts c
WHERE c.node_id = :node_id
ORDER BY c.valid_from DESC;
```

### Contract gaps
```sql
-- Nodes with no contract
SELECT n.id, n.qualified_name, n.kind, 'no_contract' AS gap_kind
FROM nodes n
LEFT JOIN contracts c ON c.node_id = n.id AND c.valid_until IS NULL
WHERE c.id IS NULL;

-- Nodes with contract but no guarantees
SELECT n.id, n.qualified_name, n.kind, 'no_guarantees' AS gap_kind
FROM nodes n
JOIN contracts c ON c.node_id = n.id AND c.valid_until IS NULL
LEFT JOIN guarantees g ON g.contract_id = c.id AND g.valid_until IS NULL
WHERE g.id IS NULL;

-- Nodes with inferred-only contracts
SELECT n.id, n.qualified_name, n.kind, 'inferred_only' AS gap_kind
FROM nodes n
JOIN contracts c ON c.node_id = n.id AND c.valid_until IS NULL
WHERE c.completeness = 'inferred';
```

### Breaking edges
```sql
SELECT e.id, e.kind, e.from_id, e.to_id
FROM edges e
WHERE e.compatibility = 'breaking';
```

### Low-confidence guarantees
```sql
SELECT g.id, g.kind, g.field_name, g.confidence, g.observations, g.violations,
       c.node_id
FROM guarantees g
JOIN contracts c ON c.id = g.contract_id
WHERE g.valid_until IS NULL          -- currently active
  AND g.observations > 100           -- enough evidence to be meaningful
  AND g.confidence < 0.95;           -- but confidence is low
```

---

## Notes

- JSONB columns (`config`, `params`) store variable-structure data that does not warrant its own table
- All foreign keys reference `nodes(id)` — the node is the anchor
- `guarantees.confidence` and `guarantees.observations` are updated by the feedback loop reading from the Observations database — they are the bridge between the two stores
- Credentials are never stored — only `credential_provider` and `credential_key` as references
- **Transaction time** (when the database recorded a fact, as distinct from when it was true) is deferred. If retroactive contract corrections become necessary, `recorded_from` / `recorded_until` columns can be added to `contracts` and `guarantees` without reworking the schema. The `valid_from` / `valid_until` foundation makes this a clean extension.
- All timestamps use `TIMESTAMPTZ` (timezone-aware). Pipelines and codebases span environments and timezones. Naive timestamps are a source of subtle bugs.
