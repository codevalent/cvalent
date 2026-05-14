# Rung 1 — Hosted CodeValent Account

**Status:** First paid product. Public launch event.
**Position:** Rung 1 of 5
**Depends on:** Rung 0 (baseline rewrite must be done — identity, store, dialect adapter, parity harness)
**Unlocks:** Rung 2 (bitemporal history) and every later rung — none of them exist without a hosted store to write into

---

## Goal

Stand up a hosted, multi-tenant store that any number of OSS installs in the same organization can push their local graphs into, and serve queries against the union over a remote MCP endpoint. The single user-visible value at this rung is **portability and cross-repo**: the user's graph is reachable from anywhere they have an API key, and queries that walk across repos in the same organization actually walk across them. Everything else is groundwork for the later rungs.

---

## Why this is its own rung

Rung 0 deliberately leaves the OSS as a single-repo local tool. Going from "single-repo local" to "multi-repo cloud-resident" is one architectural step but it crosses every layer at once: storage engine swap (SQLite → Postgres), auth model (none → org-scoped), network surface (stdio → HTTPS+API key), identity boundary (per-process → multi-tenant). Bundling any of the later rungs into this one would multiply the launch risk for no proportional revenue gain. Rung 1 ships *only* what is required to charge money.

It is also the rung with the highest distribution leverage. Every OSS install hitting a friction wall lands here. Every later rung is sold against existing paying customers; this is the only one that has to be sold against zero.

---

## What changes for the user

The user goes from "I installed cvalent" to "I am paying" in two shell commands:

```
cvalent login        # opens a browser, authenticates with Clerk, stores an API key
cvalent push         # uploads the local graph for the current repo to the hosted account
```

After `push`:
- The remote MCP endpoint at `mcp.valent.dev` (or equivalent) is reachable from any agent that has the API key.
- Queries against the remote endpoint span every repo the same account has pushed.
- `cvalent query` against `--remote` returns answers that resolve cross-repo references the local-only query previously surfaced as friction walls.
- The local OSS continues to work exactly as before. Nothing about the local install is degraded by being signed in.

---

## What gets built

**Neon Postgres instance.** Single project at launch, three logical schemas (`graph`, `obs`, `analysis`) inside one database. All three schemas exist from day one even though only `graph` is populated; this makes Rung 2 and later additive. PG18 application-time temporal columns present on all relevant tables (unused until Rung 2 but indexed and constrained correctly).

**Schema migrations.** Same migration tool, same numbering, same layout as the SQLite migrations from Rung 0. The dialect adapter from Rung 0 makes the schema definitions shared; the migrations only diverge at engine-specific syntax.

**Push protocol.** A simple HTTP/2 endpoint that accepts a serialized batch of `Node` and `edge` records produced by the OSS. Format is the in-memory `Node` structs marshaled to a stable wire format (probably Protobuf or MessagePack — pick one in this rung and freeze it). Idempotency: each push is keyed on `(account_id, repo_origin, push_id)` and replaying a push is a no-op. Pushes overwrite at this rung; bitemporal history starts in Rung 2.

**Hosted ingestor.** Server-side process that receives a push, validates identity per Model B, upserts nodes/edges into Postgres, and rebuilds the per-account adjacency cache (the EdgeCompatibility-as-property pattern from schema decision D6). Single-writer per account at launch — no concurrency story until it's needed.

**Clerk integration.** Sign-up, login, organization model, API key issuance. The "personal-attached-to-org hybrid" shape: every user has a personal account that is automatically the sole member of an organization. Multi-user organizations are valid in the data model from day one but are not exposed in the UI until Rung 3. Membership primitive is present so Rung 3 is purely additive.

**Remote MCP endpoint.** HTTPS-fronted MCP server reachable from any MCP-compatible agent (Claude Code, Cursor, custom LangGraph harnesses) by configuring the endpoint URL and an API key. Same 13 tools as the local stdio server, plus the cross-repo behavior: every query traverses the union of all pushed repos in the account. Tool responses include the same empty direction-signal fields as Rung 0.

**Single-account cross-repo resolution.** When a Rung 0 friction wall would have fired (an external import to a module that lives in another pushed repo in the same account), the hosted endpoint resolves it and returns the actual node. The friction annotation now includes a `resolved_via: hosted_account` tag so the agent can tell the difference between "we walked across" and "we still don't know."

**Query parity test, extended.** The parity harness from Rung 0 is extended to also run every query against the remote MCP endpoint connected to a Postgres test database, asserting bit-exact match on the single-repo queries and a separate snapshot for the cross-repo queries that have no local equivalent.

**Account-scoped rate limiting and basic abuse handling.** Enough to keep one runaway agent from melting the database. Not a full quota system.

**Pricing and billing wiring.** A flat-rate "Hosted Account" SKU. Stripe integration through Clerk's billing surface or directly, depending on which is less work. Single tier, generous limits, intentionally undifferentiated. The job of launch pricing is to find signal, not to capture margin.

**Public docs.** A short docs site at `valent.dev/docs` covering: install the OSS, run a local query, sign up, push, configure the remote MCP endpoint in your harness of choice, run a cross-repo query. That is the entire onboarding flow at launch.

---

## What gets populated in the graph

Same set as Rung 0 — `function` nodes, `FunctionMeta`, `Contract`, `ContractField`, `call` and `import` edges — but now centrally in Postgres, scoped per account, unioned across every pushed repo. Bitemporal columns still set to `valid_from = epoch, valid_until = NULL` and overwritten on each push.

---

## What does NOT get populated

- Any history (Rung 2)
- Multi-user data (Rung 3 — the data model supports it but the UI does not)
- Any pipeline, storage, endpoint nodes (Rung 4)
- Any execution traces (Rung 5)
- Any analysis-DB content (Rung 5)

The `obs` and `analysis` schemas exist with empty tables, indexed and constrained, ready to be written to in later rungs.

---

## MCP tool surface changes

Same 13 tools. Cross-repo resolution lights up automatically inside existing tool responses — no new tool names. The friction annotation gains a `resolved_via` discriminator. Empty direction-signal fields remain empty.

---

## Conversion trigger

This is the rung the conversion trigger from Rung 0 fires into. Every OSS install that hit a friction wall in Rung 0 — and every install that hits one for the first time after Rung 1 ships — sees a single-line annotation in its query output saying "the hosted version answers this." `cvalent login && cvalent push` is the entire conversion path. There is no separate web flow, no separate installer, no separate mental model.

---

## Definition of done

- [ ] Neon Postgres instance live with all three schemas migrated and indexed
- [ ] Push protocol stable; wire format frozen; documented
- [ ] Hosted ingestor handles a push end-to-end including identity validation and adjacency cache rebuild
- [ ] Clerk integration shipped: sign-up, login, org creation, API key issuance, billing
- [ ] Remote MCP endpoint reachable over HTTPS with API key auth
- [ ] All 13 MCP tools function against the remote endpoint
- [ ] Cross-repo queries return resolved nodes for the single-account case
- [ ] Parity harness extended; passes against both local SQLite and remote Postgres
- [ ] Rate limiting and basic abuse handling in place
- [ ] Pricing live; a real human can pay for an account
- [ ] Docs site live; install → push → cross-repo query is documented end-to-end
- [ ] At least one design partner has signed in, pushed, and run a cross-repo query against their own code

---

## Dependencies on prior rungs

- **Rung 0 identity normalization** — without Model B, two repos importing the same module produce conflicting nodes and cross-repo resolution is incoherent
- **Rung 0 dialect adapter** — without it, every query has to be written twice, once per engine
- **Rung 0 unified `Node` type** — the wire format on push is the in-memory struct; if the struct is wrong, the wire format is wrong
- **Rung 0 friction engineering** — without it, there is no conversion trigger to fire into

---

## Open questions deferred

- Multi-writer concurrency on a single account (defer until measured contention exists)
- Per-repo permissions inside an account (this is part of Rung 3, not here)
- WorkOS migration for enterprise SSO/SCIM (later, when an enterprise actually asks)
- pgvector / semantic search over the graph (not foreclosed by this rung; not built either)
- How to handle pushes from an OSS install that is older than the current schema (probably: refuse and tell the user to upgrade; revisit if it bites)

---

## Cross-references

- Worldview: § "The product line", § "Decision principles" #4 (Distribution is upgrading)
- Memory: `project_valent_launch_shape.md`, `project_valent_storage_stack.md`
- Schema: `valent_graph_schema.md` (full DDL)
