# Phase 3: Agents

**Layer**: 3 (proprietary, paid — where real charging starts)
**Depends on**: Phase 1 (graph with contracts), Phase 2 (cloud sync + history)

## What this phase adds

AI agents that work against the contract graph to find problems, gate regressions, and improve code over time. Static infrastructure parsing. Algorithmic domain detection. This is the layer that justifies paid individual subscriptions.

## Capabilities

### Agent Scans
- Scan runner: takes cloud graph + agent prompt, produces findings
- Runs nightly (async) or on-demand
- Every finding links to the production standard score — "fixing this moves you from 34% to 37%"
- Results via email, Slack, PR comment

Scan catalog (from existing agent work in base/agents/):

| Category | Scans |
|---|---|
| Code Health | Dead code, coupling, error handling, boundary safety |
| Security | Auth audit, data flow, supply chain, concurrency, resource lifecycle |
| Change Mgmt | Contract validation, breaking change detection, cost signals |
| Operations | Resilience patterns, SLO design, runbook audit |
| Architecture | Domain coherence, cross-domain coupling, implicit domain detection |

### Branch Diff + CI Gates
- `cpact diff <branch1> <branch2>` — compare graphs, surface contract changes
- Fast contract checks on every push (seconds)
- Deep scan on PR open (changed subgraph only)
- GitHub Action wrapper, GitLab CI, Bitbucket Pipelines support
- Exit codes: 0 (clean), 1 (contract breaks) for merge gates
- PR comment: "This change breaks 3 callers of validateUser"

### Dependency-Aware Test Runner
- `cpact test --changed` queries the graph to identify tests affected by changed contracts
- Runs only affected tests, skips everything else
- Value grows as test suites grow to minutes

### Domain Detection (Algorithmic)
- Community detection (Leiden or similar) over call + import graph
- Auto-label domains from function/module name patterns
- Cohesion score per domain
- Cross-domain dependency map
- Architectural drift flagging: functions that belong in a different domain based on contracts and call patterns
- Writes domain labels back to graph (SQLite migration from Phase 1 schema)

### IaC / Infrastructure Parsing (Static)
- Parse Terraform, CloudFormation, Pulumi files
- Map infrastructure resources to code that uses them
- "This Lambda function runs this code, connects to this RDS instance"
- Static only — runtime infra discovery is Phase 4

### Autonomous Repair (Late Phase 3)
- Agent finds gap → generates fix → opens PR
- Start with low-risk: dead code removal, adding type annotations
- Every repair is a PR — human stays in the loop
- Acceptance rate tracking feeds back into repair quality
- Requires established user trust from scan results

## Pricing

This is where real charging starts:
- Agent scans: included in paid individual tier
- CI gates: included in paid individual tier
- Team features (shared scan results, team dashboards): paid team tier
