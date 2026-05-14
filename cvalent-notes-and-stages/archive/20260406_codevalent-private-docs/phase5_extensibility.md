# Phase 5: Extensibility

**Layer**: 5 (proprietary, platform fees)
**Depends on**: Phases 1-4 established and generating revenue

## What this phase adds

Third-party hooks, a scan marketplace, and eventually paid lightweight alternatives to existing developer tools. This is the platform play.

## Capabilities

### Custom Scans + Marketplace
- Custom scan authoring: graph pattern queries or natural language prompts
  - Deterministic: "Flag any public function with >5 parameters and no contract"
  - LLM-powered: "Review this subgraph for [domain-specific concern]"
- Custom production standard rubrics: teams define what "production standard" means
- Marketplace: publish, discover, install community scans
  - Free community scans (visibility for authors)
  - Paid premium scans (revenue share with authors)
- Domain-specific scan packs:
  - Healthcare: "Flag any function handling DOB without encryption"
  - Fintech: "Trace all money-amount fields, flag unvalidated boundaries"
  - GDPR: "Map all PII parameters, verify deletion paths exist"
- Org-private scans: internal rules shared across team repos

### Cross-Repo Intelligence
- Dependency contract checking: "Library X changed its API in v3.2 — which functions break?"
- Pattern intelligence: "10,000 repos handle this pattern this way — yours differs"
- Architecture similarity: "Your auth module is structured like [OSS project X]"
- Ecosystem health: "This library is used by 2,000 repos but unmaintained for 18 months"
- Requires significant user base (thousands of graphs) for patterns to be meaningful

### Hooks + Integrations
- Public API for cloud graph access
- Webhooks: "When module X's contract changes, notify Slack / trigger Jenkins / update Jira"
- IDE plugins: VS Code showing "3 callers, contract: {id: string, amount: float}" inline
- Project management integration: link contract changes to Jira/Linear tickets

### Paid Lightweight Alternatives (Long-term)
- For teams that don't have enterprise tooling
- Simpler, cheaper versions of observability, infra mapping, security scanning
- Acknowledged as significantly harder than it appears — essentially building competing products
- Evaluate based on market demand signals, not speculative

## Revenue model
- Marketplace: 30% fee on paid scan revenue
- Platform API: usage-based
- Team/org features: seat-based
- IDE plugins: bundled with paid tier

## Honest assessment
- Marketplace has a cold-start problem (no scans = no users, no users = no scan authors)
- Cross-repo intelligence requires 10,000+ graphs for meaningful patterns
- This phase is gated by user base, not engineering
- Treat as directional — don't let it constrain earlier phases
