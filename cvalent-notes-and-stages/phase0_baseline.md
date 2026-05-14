# Rung 0 — Pre-launch Baseline

**Status:** Required before anything ships publicly
**Position:** Rung 0 of 5
**Depends on:** Nothing (this is the floor)
**Unlocks:** Rung 1 (Hosted Account) — none of the hosted work is safe to start until this rung is done

---

## Goal

Take the existing `cvalent` Go project and rebuild its foundation around the unified `Node` model and Model B identity, on a SQLite-backed store, with a parity-tested query surface, so that the OSS install can ship publicly *and* the hosted store can be added in Rung 1 without re-doing this work. This rung produces no new user-facing features. Its job is to make every later rung cheap.

---

## Why this is its own rung

Every later rung touches identity, storage, or query surface. If those are wrong at the OSS level, every fix downstream is a migration. The cost of getting these wrong compounds. The cost of getting them right once and never again is bounded.

Three things specifically force this rung to exist:

1. **Identity is wrong today.** The Go parser emits short package names instead of full module paths. TypeScript parser is inconsistent. Python doesn't read distribution name. Until these are fixed, two repos importing the same module will produce nodes that disagree on identity, and the moment cross-repo lands in Rung 1 the graph is incoherent.
2. **Storage is wrong today.** GoraphDB is a research-grade embedded graph DB whose query surface, durability story, and dialect compatibility do not match the long-term Postgres direction. Carrying it into the hosted rung means writing every query twice and managing two incompatible engines.
3. **The data model is wrong today.** `FunctionNode` is a flat struct, not a typed slot inside the unified `Node`. The schema notes already describe the destination shape; this rung is the migration to it.

---

## What changes for the user

Almost nothing visible. This rung rebuilds the floor. The single user-visible change is one mandatory re-init: existing `.cvalent/graph.db` files (GoraphDB) are read once by a one-shot migrator and converted to the new `.cvalent/store.db` (SQLite). Existing `cvalent query` commands return the same answers, on the same arguments, in the same shapes — verified by a parity harness.

The OSS engineered-friction surface (see below) does become visible in this rung, because that friction is part of what makes Rung 1 a real conversion event rather than a speculative upgrade.

---

## What gets built

**Unified `Node` type in Go.** A single struct with typed meta slots (`FunctionMeta` populated; `PipelineMeta`, `StorageMeta`, `EndpointMeta` defined but unused). The current `parser.FunctionNode` becomes the contents of `Node.FunctionMeta`. All parsers emit `[]Node`, not `[]FunctionNode`. The schema-notes Decision D9 mapping is the spec.

**Parser identity normalization to Model B.**
- Go parser reads `go.mod` to recover the full module path; `QualifiedName` becomes `<module>/<package>.<receiver?>.<func>`.
- Java parser already mostly correct; verified by tests.
- TypeScript parser reads the nearest `package.json` for the distribution name and emits consistent qualified names regardless of import style.
- Python parser reads `pyproject.toml` / `setup.cfg` / `setup.py` to recover the distribution name; falls back to `repo:<account>/<repo>` when there is no distribution.
- Fallback rule: when no distribution can be resolved, identity is `repo:<account>/<repo>` and the node is tagged accordingly so cross-repo merging won't accidentally collapse two unrelated functions.
- Property tests assert that two parses of the same source produce identical UUIDs, and that two repos importing the same published module produce nodes with identical UUIDs.

**SQLite store with the unified schema.** A single SQLite file at `.cvalent/store.db` containing the same logical tables that the hosted Postgres schema uses (`nodes`, `node_function_meta`, `contracts`, `contract_fields`, `guarantees`, `edges`, plus the bitemporal columns even though they are unused at this rung). Indexes match the hosted schema's indexes.

**Dialect adapter.** A thin layer that lets the same query construction code run against SQLite locally and Postgres in Rung 1. Differences are isolated: identifier quoting, recursive CTE syntax, JSON column types, upsert syntax. Everything else is shared.

**One-shot GoraphDB → SQLite migrator.** A subcommand (`cvalent migrate-store`) that reads the legacy `graph.db`, normalizes identity to Model B on the way in, and writes the new `store.db`. Idempotent. Refuses to run if the new store already exists. Documented as a one-time step in the upgrade notes.

**GoraphDB retired.** Removed from `go.mod`, all imports purged, `internal/graph/graph.go` deleted or rewritten as a SQLite wrapper with the same external interface so callers don't churn.

**Query parity harness.** A snapshot-based test that takes a fixture repo, runs every `cvalent query` command against both the legacy GoraphDB store (frozen, kept only for the harness) and the new SQLite store, and asserts bit-exact match on the JSON output. This is the only safety net that lets the rewrite ship.

**MCP server stays on stdio**, pointed at the new SQLite store, with the same 13 tools it has today. No new tools at this rung. The tool *responses*, however, gain the empty-but-visible direction-signal fields (`pipeline_references: []`, `recent_traces: []`, `contract_history: []`, `upstream_storage: []`, `downstream_storage: []`) so Rung 1 doesn't have to re-shape responses.

**OSS engineered friction.** Every query that hits a structural wall — an unresolved cross-repo reference, an external package call, a contract impact that escapes the repo — annotates the result with:
- The resolved external identity (per Model B), so the agent at least knows what it can't see
- A short, plain-text note: `cross_repo_resolution_available: hosted` (or similar) telling the user the hosted version answers this question
- A boundary tag on the affected query result so an LLM can spot the wall without parsing prose

This is engineered carefully: it must be honest (never lie about hosted features that don't exist), useful (the resolved identity is real signal even at the OSS level), and visible (the conversion trigger only fires if the user notices the wall).

**Migration tooling baseline.** `goose` (or `golang-migrate` — decide once and stick with it) wired in for the SQLite schema, with the migration directory laid out the same way the hosted Postgres schema will use in Rung 1. Same migration DSL, same numbering, same style.

---

## What gets populated in the graph

- `Node` records of kind `function` from each parsed source file
- `FunctionMeta` slot populated per Decision D9
- `Contract` and `ContractField` records derived from function signatures
- `Guarantee` records where the parser can infer them (initially: nothing — guarantees come from runtime evidence in Rung 5)
- `edges` of kind `call` and `import` between function nodes
- All bitemporal columns present, all set to `valid_from = epoch, valid_until = NULL` (history is a no-op until Rung 2)

---

## What does NOT get populated

- Any node kind other than `function`
- Cross-repo edges (the wall the friction engine surfaces)
- Pipeline, storage, or endpoint nodes
- Execution traces or contract violations
- Contract history (the columns exist; nothing writes to them yet)
- Any field outside `FunctionMeta` on the Node type

This scope guard is load-bearing: the rung exists to make later rungs cheap, not to be feature-complete.

---

## MCP tool surface changes

No new tools. Existing 13 tools all continue to work. Two changes only:
1. Tool responses gain the empty direction-signal fields described above.
2. Tool responses for cross-repo / external references gain the friction annotations described above.

The tool list is frozen at this rung. Adding tools is a Rung 1+ activity.

---

## Conversion trigger preparation

This rung doesn't yet have a paid product to convert into. What it does is engineer the OSS so that the moment Rung 1 lands, every existing OSS install is already surfacing the conversion trigger in its query output. The friction is wired up before there's anywhere to convert *to*, so that the day Rung 1 ships there is no rewrite of the OSS to point at it.

---

## Definition of done

- [ ] All four parsers emit Model B–compliant qualified names; property tests pass
- [ ] `Node` type lives in `internal/model/` and is the only structure parsers produce
- [ ] SQLite store at `.cvalent/store.db` with full unified schema
- [ ] Dialect adapter compiles against both SQLite and a Postgres test instance, even though the Postgres path is unused at this rung
- [ ] GoraphDB removed from `go.mod`; build succeeds with no references
- [ ] One-shot migrator round-trips the legacy fixture without identity drift
- [ ] Query parity harness asserts bit-exact match on every `cvalent query` command across the legacy fixture and the new store
- [ ] MCP responses include the empty direction-signal fields and the friction annotations
- [ ] Migration tooling is wired with the same numbering and layout the hosted schema will adopt
- [ ] `cvalent` builds, installs, and runs end-to-end on a fresh Linux and macOS environment from a public release artifact
- [ ] Public release artifacts (Linux + macOS, amd64 + arm64) signed and downloadable
- [ ] OSS README rewritten to describe the substrate framing — no leftover language about "DataValent" or "code contracts vs. pipeline contracts"

---

## Open questions deferred to later rungs

- How exactly the hosted push protocol formats batches (Rung 1)
- How identity collisions across distributions are surfaced to the user (Rung 1)
- Whether the SQLite store should ever accept external pushes locally (probably never; deferred indefinitely)

---

## Cross-references

- Worldview: `worldview.md` § "The product line", § "Decision principles" #6 (Postgres for everything)
- Memory: `project_valent_storage_stack.md`, `project_valent_identity_model.md`
- Schema: `valent_schema.md` D1, D9; `valent_graph_schema.md`
