# Rung 2 — Bitemporal History

**Status:** First post-launch rung. First feature that compounds value daily.
**Position:** Rung 2 of 5
**Depends on:** Rung 1 (hosted store, push protocol)
**Unlocks:** Every later rung gets to assume "history is real" — Rung 5's analysis loop is impossible without it

---

## Goal

Stop overwriting on push. Make `valid_from` / `valid_until` real. Let any query about the graph be answered "as of" any point in time. Populate `contract_history` so an agent can ask "what did this function's contract look like before yesterday's deploy" and get a concrete answer.

---

## Why this is its own rung

Two reasons. First, history is the single feature that compounds value the moment it ships and discards value every day it isn't shipped — every push without history is a permanent loss of signal. Shipping it second maximizes the recovered signal over the product's lifetime.

Second, history is technically isolated. The columns are already in the schema (Rung 0/1 wired them up unused). The push ingestor is the only thing that has to change. The query layer changes additively: new "as-of" parameters on existing queries, new history-walking tools. No architectural rewrites. This makes it the cheapest large-value rung to ship after launch.

It is also chosen ahead of teams (Rung 3) because the launch-era buyer is an individual or two-person platform team. Team pain isn't immediate. History pain is — "I broke something, what did it look like before" is an emotional trigger that hits on day one.

---

## What changes for the user

**New: as-of queries.** Every existing MCP tool gains an optional `as_of: <timestamp>` parameter. With it, the tool returns the graph as it existed at that moment. Without it, behavior is unchanged.

**New: history-walking tools.** A small number of new MCP tools specifically for walking history:
- `contract_history(node_id, since?, until?)` — every version of a function's contract in a window
- `node_change_log(node_id, since?, until?)` — every state change on a node
- `diff_at(node_id, t1, t2)` — symbolic diff of a node between two times

**Push behavior changes silently.** A push no longer overwrites. It writes a new bitemporal version of any node/edge/contract that has changed, and closes out the previous version. Pushes that change nothing produce no new history rows. The user does not have to do anything to opt into this; the behavior just becomes correct.

**`recent_traces` and the other empty fields stay empty.** Only `contract_history` lights up at this rung.

---

## What gets built

**Bitemporal write path in the ingestor.** On each push, the ingestor compares the incoming batch against the current valid version of each affected node/edge/contract. For each genuine change, it sets `valid_until = now()` on the previous version and inserts a new row with `valid_from = now(), valid_until = NULL`. Identity (the composite key) does not change across versions; only the `(EnvironmentID, Kind, QualifiedName, valid_from)` row identity changes.

**SQL:2011 / PG18 application-time temporal constraints turned on.** Primary keys and foreign keys become temporal. Overlap exclusion constraints prevent two valid rows for the same logical identity at the same instant.

**Indexes for as-of queries.** Each historical table gains a `(qualified_name, valid_from DESC)` index, plus a partial index on `valid_until IS NULL` for the hot "current" path. These keep the current-state query at Rung 1 performance while making historical queries cheap.

**Recursive CTEs gain a temporal dimension.** Edge traversal queries (callers, impact, etc.) take an optional timestamp and walk the version of the graph that was valid at that time. The dialect adapter handles the SQL differences between SQLite and Postgres. (SQLite gets the same logical behavior; the OSS doesn't need it for cross-repo, but it does need it for the query parity harness.)

**`contract_history` field starts populating.** The empty direction-signal field from Rung 1 becomes a non-empty list on response when the queried node has a history. Older clients reading the field as empty don't break.

**History compaction policy (decided, not necessarily built yet).** A documented policy on how long full history is retained at full resolution and what (if anything) is downsampled or archived later. Default at this rung: keep everything forever; revisit when the cost of "everything" is measured to hurt.

**Migration of existing accounts.** Any account that pushed in Rung 1 has its current state retroactively stamped with `valid_from = first_push_time, valid_until = NULL`. No retroactive history is fabricated; history starts at this rung for them.

**OSS friction update.** The OSS continues not to track history (single-repo, single-time, no compounding signal). Friction annotations now mention "the hosted version remembers this over time" when relevant. The OSS does not get history at this rung or any later rung.

---

## What gets populated in the graph

Same set of node and edge kinds as Rung 1, plus:
- Multiple temporal versions of any node/edge/contract that has changed
- `contract_history` rows for every contract that has changed
- `node_state_changes` rows in the `obs` schema (this is the first table outside `graph` to receive writes)

---

## What does NOT get populated

- Pipeline / storage / endpoint nodes (Rung 4)
- Execution traces (Rung 5)
- Anything in the `analysis` schema (Rung 5)
- Multi-user state (Rung 3 — though again, the data model already supports it)

---

## MCP tool surface changes

- All existing tools gain optional `as_of` parameter (additive, non-breaking)
- Three new tools: `contract_history`, `node_change_log`, `diff_at`
- `contract_history` direction-signal field begins returning non-empty values

---

## Conversion trigger

For existing paying customers: this rung is the first proof that the substrate compounds. The pitch from the moment of sign-up was "we accumulate signal." This rung is when that pitch starts being true.

For OSS users: friction annotations now also include the temporal dimension ("the hosted version remembers this over time"), reinforcing the original conversion trigger from Rung 0/1.

---

## Definition of done

- [ ] Push ingestor writes bitemporally; existing test coverage extended to assert version chains
- [ ] Application-time temporal constraints active on all relevant tables
- [ ] As-of indexes in place; query plans for current-state queries unchanged from Rung 1
- [ ] All existing MCP tools accept and honor `as_of`
- [ ] `contract_history`, `node_change_log`, `diff_at` tools shipped and documented
- [ ] `contract_history` direction-signal field populated
- [ ] Existing accounts migrated cleanly; the migration is a one-shot, idempotent script
- [ ] History compaction policy documented (even if no compaction is built yet)
- [ ] At least one paying customer has run an as-of query against a real change in their own graph and gotten the answer they expected
- [ ] Documentation updated: as-of semantics, history tools, what is and isn't recorded

---

## Dependencies on prior rungs

- **Rung 1 hosted store** — bitemporal needs centralized writes; OSS local SQLite doesn't get this
- **Rung 1 push protocol** — the push contract has to be stable enough that the ingestor can diff incoming against current

---

## Open questions deferred

- Whether to expose history retention as a per-account setting
- Whether to allow users to "rewrite" history (probably never; deferred)
- How history interacts with multi-environment / staging vs prod (Rung 4 territory)
- Cross-environment time alignment (Rung 4–5 territory)

---

## Cross-references

- Worldview: § "The development path: rungs, not phases", § "Decision principles" #5 (Reliability over completeness)
- Memory: `project_valent_launch_shape.md` (rationale for history-before-teams)
- Schema: `valent_graph_schema.md` (bitemporal columns), `valent_observations_schema.md` (`node_state_changes`)
