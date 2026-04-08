-- +goose Up
-- +goose StatementBegin
--
-- Rung 0 Postgres schema. Mirrors the SQLite schema (same logical
-- columns, same indices) but uses PG18-style temporal primary keys
-- (PRIMARY KEY (id, valid_from WITHOUT OVERLAPS)) so that bitemporal
-- write semantics can land at Rung 2 with zero schema work.
--
-- No data path exists in Rung 0 — this file ships so that Rung 1's
-- hosted store starts with the schema already defined and Rung 0's
-- parity harness can run against an ephemeral Neon branch.
--

CREATE TABLE nodes (
    id              UUID        NOT NULL,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until     TIMESTAMPTZ NULL,
    environment     TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    qualified_name  TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    distribution    TEXT        NOT NULL,
    module_path     TEXT        NOT NULL,
    language        TEXT        NOT NULL,
    file            TEXT        NOT NULL,
    identity_source TEXT        NOT NULL,
    source          TEXT        NOT NULL DEFAULT 'parsed',
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_nodes_logical          ON nodes(id, valid_from DESC);
CREATE INDEX idx_nodes_current          ON nodes(id) WHERE valid_until IS NULL;
CREATE INDEX idx_nodes_qualified_name   ON nodes(qualified_name) WHERE valid_until IS NULL;
CREATE INDEX idx_nodes_distribution     ON nodes(distribution) WHERE valid_until IS NULL;

CREATE TABLE node_function_meta (
    node_id              UUID        NOT NULL,
    valid_from           TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until          TIMESTAMPTZ NULL,
    start_line           INTEGER     NOT NULL,
    end_line             INTEGER     NOT NULL,
    exported             BOOLEAN     NOT NULL,
    tag                  TEXT        NOT NULL,
    receiver             TEXT        NOT NULL DEFAULT '',
    pointer_receiver     BOOLEAN     NOT NULL DEFAULT FALSE,
    params               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    returns              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    contract_completeness TEXT       NOT NULL DEFAULT 'inferred',
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (node_id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_function_meta_current ON node_function_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_pipeline_step_meta (
    node_id      UUID        NOT NULL,
    valid_from   TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until  TIMESTAMPTZ NULL,
    step_kind    TEXT        NOT NULL DEFAULT '',
    step_meta    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (node_id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_pipeline_step_meta_current ON node_pipeline_step_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_storage_meta (
    node_id      UUID        NOT NULL,
    valid_from   TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until  TIMESTAMPTZ NULL,
    storage_kind TEXT        NOT NULL DEFAULT '',
    storage_meta JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (node_id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_storage_meta_current ON node_storage_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_endpoint_meta (
    node_id       UUID        NOT NULL,
    valid_from    TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until   TIMESTAMPTZ NULL,
    endpoint_kind TEXT        NOT NULL DEFAULT '',
    endpoint_meta JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (node_id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_endpoint_meta_current ON node_endpoint_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE contracts (
    node_id      UUID        NOT NULL,
    valid_from   TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until  TIMESTAMPTZ NULL,
    completeness TEXT        NOT NULL DEFAULT 'inferred',
    body         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (node_id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_contracts_current ON contracts(node_id) WHERE valid_until IS NULL;

CREATE TABLE contract_fields (
    node_id     UUID        NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until TIMESTAMPTZ NULL,
    direction   TEXT        NOT NULL,
    position    INTEGER     NOT NULL,
    name        TEXT        NOT NULL,
    type        TEXT        NOT NULL,
    nullable    BOOLEAN     NOT NULL DEFAULT FALSE,
    PRIMARY KEY (node_id, valid_from, direction, position)
);

CREATE INDEX idx_contract_fields_current ON contract_fields(node_id) WHERE valid_until IS NULL;

CREATE TABLE guarantees (
    id          UUID        NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until TIMESTAMPTZ NULL,
    node_id     UUID        NOT NULL,
    kind        TEXT        NOT NULL,
    body        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_guarantees_node    ON guarantees(node_id) WHERE valid_until IS NULL;
CREATE INDEX idx_guarantees_current ON guarantees(id) WHERE valid_until IS NULL;

CREATE TABLE edges (
    id          UUID        NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    valid_until TIMESTAMPTZ NULL,
    from_node   UUID        NOT NULL,
    to_node     UUID        NOT NULL,
    kind        TEXT        NOT NULL,
    source      TEXT        NOT NULL DEFAULT 'parsed',
    meta        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    PERIOD FOR system_time (valid_from, valid_until),
    PRIMARY KEY (id, valid_from WITHOUT OVERLAPS)
);

CREATE INDEX idx_edges_logical    ON edges(id, valid_from DESC);
CREATE INDEX idx_edges_current    ON edges(id) WHERE valid_until IS NULL;
CREATE INDEX idx_edges_from       ON edges(from_node) WHERE valid_until IS NULL;
CREATE INDEX idx_edges_to         ON edges(to_node) WHERE valid_until IS NULL;
CREATE INDEX idx_edges_kind       ON edges(kind) WHERE valid_until IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS edges;
DROP TABLE IF EXISTS guarantees;
DROP TABLE IF EXISTS contract_fields;
DROP TABLE IF EXISTS contracts;
DROP TABLE IF EXISTS node_endpoint_meta;
DROP TABLE IF EXISTS node_storage_meta;
DROP TABLE IF EXISTS node_pipeline_step_meta;
DROP TABLE IF EXISTS node_function_meta;
DROP TABLE IF EXISTS nodes;
-- +goose StatementEnd
