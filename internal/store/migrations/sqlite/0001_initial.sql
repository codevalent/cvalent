-- +goose Up
-- +goose StatementBegin
--
-- Rung 0 SQLite schema. This is the long-term-safe shape: bitemporal
-- columns, temporal primary keys, all four meta tables defined empty,
-- source columns on nodes and edges. Rungs 2 (bitemporal write
-- semantics) and 4 (pipeline ingest) require zero ALTER TABLE work
-- against this schema.
--
-- Conventions:
--   * Every graph table has valid_from and valid_until.
--   * valid_from defaults to epoch (1970-01-01) at Rung 0; Rung 2
--     introduces real timestamps on write.
--   * valid_until = NULL means "currently valid".
--   * source defaults to 'parsed'; Rung 4 introduces 'openlineage',
--     'declared', 'discovered'.
--   * kind is TEXT with no CHECK constraint — application enforces.
--   * params, returns, and edge meta are stored as JSON TEXT.
--

CREATE TABLE nodes (
    id              BLOB    NOT NULL,
    valid_from      TEXT    NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until     TEXT    NULL,
    environment     TEXT    NOT NULL,
    kind            TEXT    NOT NULL,
    qualified_name  TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    distribution    TEXT    NOT NULL,
    module_path     TEXT    NOT NULL,
    language        TEXT    NOT NULL,
    file            TEXT    NOT NULL,
    identity_source TEXT    NOT NULL,
    source          TEXT    NOT NULL DEFAULT 'parsed',
    PRIMARY KEY (id, valid_from)
);

CREATE INDEX idx_nodes_logical          ON nodes(id, valid_from DESC);
CREATE INDEX idx_nodes_current          ON nodes(id) WHERE valid_until IS NULL;
CREATE INDEX idx_nodes_qualified_name   ON nodes(qualified_name) WHERE valid_until IS NULL;
CREATE INDEX idx_nodes_distribution     ON nodes(distribution) WHERE valid_until IS NULL;

CREATE TABLE node_function_meta (
    node_id              BLOB NOT NULL,
    valid_from           TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until          TEXT NULL,
    start_line           INTEGER NOT NULL,
    end_line             INTEGER NOT NULL,
    exported             INTEGER NOT NULL,
    tag                  TEXT NOT NULL,
    receiver             TEXT NOT NULL DEFAULT '',
    pointer_receiver     INTEGER NOT NULL DEFAULT 0,
    params               TEXT NOT NULL DEFAULT '[]',
    returns              TEXT NOT NULL DEFAULT '{}',
    contract_completeness TEXT NOT NULL DEFAULT 'inferred',
    PRIMARY KEY (node_id, valid_from)
);

CREATE INDEX idx_function_meta_current ON node_function_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_pipeline_step_meta (
    node_id      BLOB NOT NULL,
    valid_from   TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until  TEXT NULL,
    step_kind    TEXT NOT NULL DEFAULT '',
    step_meta    TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id, valid_from)
);

CREATE INDEX idx_pipeline_step_meta_current ON node_pipeline_step_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_storage_meta (
    node_id      BLOB NOT NULL,
    valid_from   TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until  TEXT NULL,
    storage_kind TEXT NOT NULL DEFAULT '',
    storage_meta TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id, valid_from)
);

CREATE INDEX idx_storage_meta_current ON node_storage_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE node_endpoint_meta (
    node_id       BLOB NOT NULL,
    valid_from    TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until   TEXT NULL,
    endpoint_kind TEXT NOT NULL DEFAULT '',
    endpoint_meta TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id, valid_from)
);

CREATE INDEX idx_endpoint_meta_current ON node_endpoint_meta(node_id) WHERE valid_until IS NULL;

CREATE TABLE contracts (
    node_id    BLOB NOT NULL,
    valid_from TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until TEXT NULL,
    completeness TEXT NOT NULL DEFAULT 'inferred',
    body         TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id, valid_from)
);

CREATE INDEX idx_contracts_current ON contracts(node_id) WHERE valid_until IS NULL;

CREATE TABLE contract_fields (
    node_id     BLOB NOT NULL,
    valid_from  TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until TEXT NULL,
    direction   TEXT NOT NULL,
    position    INTEGER NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    nullable    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (node_id, valid_from, direction, position)
);

CREATE INDEX idx_contract_fields_current ON contract_fields(node_id) WHERE valid_until IS NULL;

CREATE TABLE guarantees (
    id          BLOB NOT NULL,
    valid_from  TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until TEXT NULL,
    node_id     BLOB NOT NULL,
    kind        TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, valid_from)
);

CREATE INDEX idx_guarantees_node    ON guarantees(node_id) WHERE valid_until IS NULL;
CREATE INDEX idx_guarantees_current ON guarantees(id) WHERE valid_until IS NULL;

CREATE TABLE edges (
    id          BLOB NOT NULL,
    valid_from  TEXT NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    valid_until TEXT NULL,
    from_node   BLOB NOT NULL,
    to_node     BLOB NOT NULL,
    kind        TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'parsed',
    meta        TEXT NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, valid_from)
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
