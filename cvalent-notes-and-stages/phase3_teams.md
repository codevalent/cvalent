# Rung 3 — Teams and Sharing

**Status:** First "this is a real org tool" rung
**Position:** Rung 3 of 5
**Depends on:** Rung 1 (Clerk + org/membership primitive present from day one)
**Unlocks:** Enterprise conversations; multi-seat pricing; the broader user base required for Rung 5's cross-customer signal

---

## Goal

Make the org-scoped substrate built in Rung 1 actually usable by more than one human. Add roles, shared graph views, multi-user push, audit trails, and enough surface for a small platform team to live inside the product daily without stepping on each other.

This rung is purely additive at the data layer because the membership primitive was wired in at Rung 1. The work is in UI, auth flows, permissions, and the governance behaviors that turn a single-user account into a team product.

---

## Why this is its own rung

Two reasons it isn't bundled with Rung 1.

First, the launch-era buyer is one person or a two-person platform team. Building team features at launch puts substantial scope (roles, permissions, invites, audit, settings UIs, account management) on the critical path with no proportional revenue gain.

Second, the team rung is only valuable once the substrate compounds. Sharing an empty graph is uninteresting. Sharing a graph that has a month of history (Rung 2), or pipeline data (Rung 4), is the actual product. So Rung 3 sits where it does because it is most valuable when there is something worth sharing.

It is not Rung 4 because pipeline ingest is the rung that puts Valent into a fundamentally new conversation with buyers; teams is still inside the conversation Rung 1 started. Pipeline ingest is the bigger door, so it goes later.

---

## What changes for the user

- **Invite a teammate.** A user can invite another user into their organization by email; the invitee accepts via Clerk and lands inside the same org with the same shared graph.
- **Roles.** A small role set (initially `owner`, `admin`, `member`, `viewer`) controls who can push, who can manage members and billing, and who can only query.
- **Shared graph views.** Every member of an org sees the same union of pushed repos. There is one graph per org. There are no private subgraphs at this rung.
- **Audit trail.** Every push and every membership change is recorded with `(actor, action, target, timestamp)` so the org can see who did what.
- **Per-member API keys.** API keys are issued per user, not per org. Revoking a user revokes their keys. This is what makes "remove a teammate" actually work.
- **Account management UI.** A small web surface (the first one Valent ships outside the docs site) for invites, role changes, billing, and viewing audit history. This is the first rung that has a UI worth opening — until now everything has been CLI + MCP.

---

## What gets built

**Clerk org features fully wired.** Multi-user orgs, invitations, role assignment, membership lifecycle. Clerk handles the auth and identity primitives; Valent maps them to its own role system at the API boundary.

**Permission middleware.** Every push, every MCP query, and every account-management action goes through a single permission check that resolves `(user, org, action) → allow/deny`. Centralizing this is what keeps the rule set auditable. Decentralizing it is how authorization bugs ship.

**Per-user API key issuance and revocation.** Each user can mint, name, and revoke API keys scoped to their own membership. Revoking a key terminates any in-flight MCP session it authenticates.

**Audit log table in `graph` schema.** A simple append-only `audit_events` table partitioned by month. Every mutating action — push, invite, role change, billing change, key issuance, key revocation — writes a row. Queryable through a `recent_audit_events` MCP tool and the web UI.

**Web UI (minimal).** A small Next.js / Remix / whatever surface, served at `app.valent.dev`. Pages: login, organization home (members and roles), billing, audit log, API keys. No graph visualization at this rung; the graph is consumed via MCP, not a web UI. This UI exists only because the things it manages cannot reasonably be CLI-only.

**Migration of existing accounts.** Every Rung 1/2 account already has exactly one user mapped to one org per the personal-attached-to-org hybrid. No data migration is required; the new role system defaults the existing user to `owner` and the rest comes online additively.

**Pricing model evolution (small).** Per-seat dimension added to the existing Hosted Account SKU. Still flat and generous. Goal remains signal, not margin.

**Documentation.** Updated docs cover invites, role semantics, audit, key management. The CLI gains `cvalent members`, `cvalent keys`, etc., for the CLI-first users who would rather not open the UI.

---

## What gets populated in the graph

No new node or edge kinds. The `graph` schema gains the `audit_events` table and the membership-related auxiliary tables Clerk syncs into. The substrate itself — the function graph — is unchanged at this rung. This rung is governance, not data.

---

## What does NOT get populated

- Pipeline / storage / endpoint nodes (Rung 4)
- Execution traces (Rung 5)
- Analysis schema (Rung 5)
- Any per-repo or per-subgraph permissioning (deferred indefinitely; the org is the unit of sharing)

---

## MCP tool surface changes

- One new tool: `recent_audit_events(since?, until?, actor?, action?)`
- Existing tools become permission-aware: a `viewer` cannot push; a `member` can push but not change membership; etc. This is enforced at the API layer, transparent to existing tool clients except for new permission-denied responses.

---

## Conversion trigger

This rung doesn't have an OSS-to-paid trigger; everyone here is already paying. The trigger is *seat expansion*: a paying user invites a teammate, the teammate joins, and the seat count goes up. The product becomes harder to leave with every additional person inside it.

It also creates the first plausible enterprise conversation, which feeds the (post-Rung-5) WorkOS migration path for SSO/SCIM.

---

## Definition of done

- [ ] Clerk multi-user orgs live; invitations, accept/decline flows working
- [ ] Role system (`owner`, `admin`, `member`, `viewer`) wired into permission middleware
- [ ] Permission middleware single-source-of-truth for every mutating endpoint and every MCP tool
- [ ] Per-user API keys: mint, list, name, revoke
- [ ] Audit log writes on every mutating action; `recent_audit_events` tool returns them
- [ ] Web UI shipped: members, billing, audit, keys
- [ ] CLI parity for the CLI-first users
- [ ] Per-seat billing live
- [ ] Docs updated
- [ ] At least one paying account with two real humans pushing from two real machines

---

## Dependencies on prior rungs

- **Rung 1 Clerk integration and membership primitive** — without it, this rung is a rewrite
- **Rung 2 history** — sharing is most valuable when there's something accumulated to share

---

## Open questions deferred

- SSO / SCIM (later, when an enterprise asks; WorkOS migration path)
- Per-repo permissions inside an org (probably never; org is the unit of sharing)
- Audit log retention policy (default forever; revisit if measured to hurt)
- A real graph visualization in the web UI (deferred; MCP is the consumption surface)

---

## Cross-references

- Worldview: § "The product line"
- Memory: `project_valent_launch_shape.md` (rationale for teams-after-history)
- Schema: `valent_graph_schema.md`
