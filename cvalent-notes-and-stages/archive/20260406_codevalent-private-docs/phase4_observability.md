# Phase 4: Observability

**Layer**: 4 (proprietary, paid)
**Depends on**: Phase 1 (contract graph as reference model), Phase 2 (cloud + history), Phase 3 (agents for analysis)
**Endgame**: This is the long-term product identity — contracts verified against production behavior.

## What this phase adds

Runtime data overlaid on the structural contract graph. The shift from "what does your code promise?" to "does your code keep those promises in production?"

## Strategy: Two paths

### Primary (A): Integration with existing APM
- Connect to Datadog, New Relic, OpenTelemetry, Jaeger
- Read aggregated trace data (span summaries, not raw spans)
- Overlay call frequency, latency, error rates on the contract graph
- `cpact connect apm --provider opentelemetry --endpoint <url>`
- No competing product — consume what they already output

### Secondary (B): Lightweight collector
- For teams without APM (~$100-150/month)
- Thin runtime agent collecting: function call counts, latency, error rates
- Not a dashboard, not alerting — just enough data to feed contract verification
- Feeds the same graph overlay as the integration path
- If team later adopts Datadog, switch to integration path seamlessly

## Capabilities

### Contract Verification in Production
- Match observed runtime data shapes against static contracts
- "Your contract says amount is always a float. In production, 0.3% of calls pass null."
- "Your contract says this function returns in < 100ms. P99 is 847ms."
- Flag violations before they become incidents

### Live PII Flow Mapping
- Completes Phase 2's PII lineage: static paths (Phase 2) + actual observed flow (Phase 4)
- "Patient DOB enters at endpoint X, flows through 4 functions, reaches logging without masking — 12,000 times per day"
- Deletion path verification: does the GDPR endpoint actually reach all observed PII sinks?

### Usage-Informed Impact Analysis
- Structural graph says "47 callers affected." Runtime says which of those 47 are exercised in production, how often, and with what data.
- "This function handles 50K requests/day in checkout" vs "this runs once in a nightly batch — safe to refactor"
- Dead path identification: structural callers that are never exercised

### Runtime Infrastructure Discovery
- Which functions write to which tables, with observed frequency
- External API calls: which functions call which third-party services
- Queue/event producers and consumers with message shapes
- "If this database goes down, these 14 functions fail, affecting these 3 user-facing endpoints"

## Competitive positioning

Not a Datadog replacement. A contract context layer on top of existing APM investment.

Datadog: "Endpoint /checkout is slow."
cpact: "Endpoint /checkout is slow because validateInventory is called 3 times per request due to a missing cache, and changing it affects 4 other endpoints — none of which are slow because they call it once. Your contract says this function is idempotent. Runtime confirms it. The fix is safe."

## Prerequisites
- Phase 1 structural graph as reference model
- Mainstream OpenTelemetry adoption (already happening)
- Significant revenue from Phases 1-3
- 10,000+ active repos for pattern density (Phase 5 cross-repo intelligence)
