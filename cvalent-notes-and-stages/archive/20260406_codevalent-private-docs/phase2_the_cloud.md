# Phase 2: The Cloud

**Layer**: 2 (proprietary, per-repo pricing)
**Depends on**: Phase 1 (complete local graph with correct edges)

## Phase 2 additions from Phase 1 planning
- Windows support (CGo + tree-sitter cross-compilation via zig cc)
- Enterprise/ directory structure (BSL 1.1, build tags, init()-based registry)
- Marketing content from real-repo showcase analysis

## What this phase adds

Cloud sync, persistent history, trend tracking, library threat detection, language expansion, and the production standard report. This is where registration lives. This is where the free→paid boundary starts.

## Capabilities

### Cloud Sync + Storage
- Graph metadata syncs to cloud on push (git hook) or on build
- Cloud API: repo registration, graph push (full + delta), API key auth
- Postgres for multi-tenant graph storage
- Anonymous key on `cpact init` → email registration to keep history
- Query telemetry: what tools get used, how often, by which languages (anonymized)

### Privacy Modes
- `full` (default) — real names sync
- `hashed` — identifiers SHA-256 hashed before leaving machine, topology preserved
- `topology-only` — names hashed, types generalized, minimal metadata
- Local queries always use real names regardless of privacy mode

### Language Expansion
- Driven by demand signals (skipped-file counts from Phase 1, GitHub issues)
- Expected waves:

| Wave | Languages | Trigger |
|---|---|---|
| Wave 1 | JavaScript, Rust, Java | High demand, clean contract extraction |
| Wave 2 | Kotlin, Swift, C# | Full types, clean extraction |
| Wave 3 | Ruby | Sorbet hints, partial contracts |
| Wave 4 | C++ | Headers + implementation, complex |
| Wave 5 | Elixir, others | Demand-driven |

### Library Threat Detection
- Dependency graph edges: your code → external library → known CVE database
- Static analysis — reads import graph from Phase 1, checks against vulnerability feeds
- Alert: "lodash@4.17.20 has CVE-XXXX, used by 14 functions in your repo"

### Production Standard Report
- Rubric: contract coverage, data lineage, dead code, error handling, test coverage, interface stability
- Free tier: snapshot score + top 5 gaps
- Paid tier: full gap list + prioritized fix roadmap + 6-month trend + shareable link
- "At this rate: production standard in X months" — the number for leadership

### PII Path Tracking (paid)
- Label parameters/types as PII during contract extraction
- `cpact query pii-paths` — all paths where PII-typed data flows through the call graph
- Requires contract data flow edges from Phase 1 — layers on clean

### Test Type Classification
- Framework-aware detection: pytest fixtures, Jest mocks, Go test helpers
- Classify as unit/integration/e2e based on framework conventions + directory structure
- Enriches Phase 1's binary test/application tag

### Onboarding Funnel
- Stage 1: `cpact init` issues anonymous API key instantly, no email
- Stage 2: Day 5 nudge via MCP response notice, day 7 key expiry
- Stage 3: Paid conversion — report generation is the trigger
- Nudges remotely configurable, A/B testable

### Incremental Parsing
- Changed file → re-parse that file only → diff edges → upsert/delete in SQLite
- Enables fast watch mode on large repos where full rebuild is too slow

## Pricing

- Cloud sync: free for up to 3 repos
- Beyond 3 repos: paid per-repo
- Production standard report (full): paid
- PII tracking: paid
- Library threat detection: paid
- Trend history: paid

## Infrastructure

| Component | Technology | Purpose |
|---|---|---|
| API server | Go service | Key mgmt, sync, telemetry, nudges, reports |
| Database | Postgres | Multi-tenant graph storage, telemetry, accounts |
| Auth | Anonymous keys → email registration | Gate paid features |
| Object storage | S3/GCS | Historical graph snapshots (paid) |
