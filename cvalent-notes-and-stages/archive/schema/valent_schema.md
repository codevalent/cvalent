# Valent Unified Graph Schema
**Version**: 0.3  
**Status**: Draft  
**Date**: April 2026

---

## Core Framing

Data moves and transforms through functions, infrastructure, and pipelines. These are not different things — they are the same thing at different levels of abstraction. One graph, one model.

**Every node** takes data in, does something to it, and puts data out.

**A contract** is the agreement about what a node takes in and puts out — the shape of the data.

**A guarantee** is a test of that agreement — a specific promise about what must be true.

**An execution trace** is the evidence — did the guarantee hold when the node actually ran?

A node does not require a contract. A contract does not require guarantees. The system captures what exists and surfaces what is missing. A good agent can take it from there.

```
Contract    →  the agreement (what flows in and out)
Guarantee   →  the test of that agreement (what must be true about it)
Trace       →  the evidence (did it hold this time?)
Violation   →  the failure (here is what we saw instead)
```

---

## Model Overview

The model has eight core types:

| Type | Purpose |
|---|---|
| `Node` | A transformation or storage unit |
| `Edge` | A data flow connection between nodes |
| `Contract` | The agreement about what a node takes in and puts out |
| `Guarantee` | A specific test of that agreement |
| `GuaranteeOrigin` | Where the guarantee came from |
| `ExecutionTrace` | An observed runtime execution of a node |
| `ContractViolation` | A recorded divergence between promise and reality |
| `ContractGap` | A missing or incomplete contract — surfaces what the agent should address |

---

## Node

Every node in the graph shares a common core. Kind-specific fields are carried in a typed metadata slot. A contract is optional — its absence is surfaced as a `ContractGap`, not an error.

```go
type Node struct {
    // Identity
    ID            string      // UUID, deterministic from (EnvironmentID, Kind, QualifiedName)
    Kind          NodeKind
    Name          string      // human readable: "ProcessOrder"
    QualifiedName string      // fully qualified within environment: "order/ProcessOrder"
    EnvironmentID string      // which system this lives in

    // Contract — optional. nil means no contract exists yet.
    Contract      *Contract

    // Operational state
    State         NodeState

    // Trigger — what starts this node
    Trigger       Trigger

    // Execution context — where and how it runs
    Engine        Engine

    // Source — how we learned about this node
    Source        NodeSource

    // Kind-specific fields — only one populated per node
    FunctionMeta  *FunctionMeta   // Kind == "function"
    PipelineMeta  *PipelineMeta   // Kind == "pipeline_step"
    StorageMeta   *StorageMeta    // Kind == "table", "stream", "bucket"
    EndpointMeta  *EndpointMeta   // Kind == "endpoint"

    // Timestamps
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### NodeKind

```go
type NodeKind string

const (
    // Transformation nodes — process data
    KindFunction     NodeKind = "function"       // code function/method
    KindQuery        NodeKind = "query"           // SQL, dbt model
    KindPipelineStep NodeKind = "pipeline_step"   // Source/Compute/Destination
    KindMLModel      NodeKind = "ml_model"        // training or inference

    // Storage nodes — hold data
    KindTable        NodeKind = "table"           // DB table, BigQuery dataset
    KindStream       NodeKind = "stream"          // Kafka, Kinesis, Pub/Sub
    KindBucket       NodeKind = "bucket"          // S3, GCS, ADLS
    KindEndpoint     NodeKind = "endpoint"        // REST API, webhook

    // Context nodes — define the execution world
    KindEnvironment  NodeKind = "environment"     // codebase, AWS account, GCP project
    KindConnector    NodeKind = "connector"       // locked pattern for connecting
    KindSchedule     NodeKind = "schedule"        // cron, event trigger
)
```

### NodeState

```go
type NodeState string

const (
    NodeStateUnknown      NodeState = "unknown"       // not yet researched
    NodeStateExperimental NodeState = "experimental"  // tried, not yet confirmed
    NodeStateLocked       NodeState = "locked"        // validated, stable
    NodeStateBroken       NodeState = "broken"        // was locked, now failing
)
```

For code functions, state is always implicitly `locked` — the language runtime is the connector and it is always trusted. State is explicit and variable for pipeline nodes.

### Trigger

```go
type Trigger struct {
    Kind     TriggerKind
    Schedule string      // cron expression if Kind == "schedule"
    EventID  string      // source node ID if Kind == "event"
}

type TriggerKind string

const (
    TriggerFunctionCall TriggerKind = "function_call"  // called by another function
    TriggerSchedule     TriggerKind = "schedule"       // time-based
    TriggerEvent        TriggerKind = "event"          // queue message, webhook
    TriggerManual       TriggerKind = "manual"         // human-initiated
    TriggerStream       TriggerKind = "stream"         // continuous/streaming
)
```

### Engine

```go
type Engine struct {
    Language string  // "go", "python", "sql", "java", ""
    Runtime  string  // "native", "lambda", "cloud_run", "container", "dbt", "spark"
    Version  string  // "1.25", "3.11", "spark-3.4", ""
}
```

`Kind` answers what a node does conceptually. `Engine` answers what runs it. They are orthogonal.

Examples:
- Go function: `{ Language: "go", Runtime: "native", Version: "1.25" }`
- dbt model: `{ Language: "sql", Runtime: "dbt", Version: "1.7" }`
- Python Lambda: `{ Language: "python", Runtime: "lambda", Version: "3.11" }`

### NodeSource

```go
type NodeSource string

const (
    NodeSourceParsed      NodeSource = "parsed"       // extracted from source code by tree-sitter
    NodeSourceOpenLineage NodeSource = "openlineage"  // received from external tool event
    NodeSourceDeclared    NodeSource = "declared"     // explicitly defined in config
    NodeSourceDiscovered  NodeSource = "discovered"   // inferred from execution traces
)
```

---

## Kind-Specific Metadata

Each kind requires a typed metadata struct. A node fails validation if its meta is nil for its kind.

### FunctionMeta
*Required for `KindFunction`*

```go
type FunctionMeta struct {
    Language   string   // "go", "python", "java", "typescript"
    Package    string   // package or module name
    Receiver   string   // for methods: "OrderRequest", "*OrderRequest"
    IsMethod   bool     // true if this is a method on a type
    StartLine  int
    EndLine    int
    Exported   bool
    Tag        string   // "application" or "test"
}
```

This is the existing CodeValent `FunctionNode` data, relocated to a typed slot. No information is lost.

### PipelineMeta
*Required for `KindPipelineStep`*

```go
type PipelineMeta struct {
    Role           string         // "source", "compute", "destination"
    ConnectorID    string         // which connector handles this step
    ConnectorState NodeState      // state of the connector pattern
    Credentials    CredentialRef  // reference only — credentials never stored
    Config         map[string]string
}

type CredentialRef struct {
    Provider string  // "aws_secrets", "vault", "env", "gcp_secrets"
    Key      string  // the key/path to look up — not the value
}
```

### StorageMeta
*Required for `KindTable`, `KindStream`, `KindBucket`*

```go
type StorageMeta struct {
    System        string  // "postgres", "bigquery", "kafka", "s3", "snowflake"
    Database      string
    Schema        string
    TableName     string
    Format        string  // "parquet", "avro", "json", "csv", "delta"
    PartitionKey  string
    RetentionDays int
}
```

### EndpointMeta
*Required for `KindEndpoint`*

```go
type EndpointMeta struct {
    Protocol   string  // "http", "grpc", "graphql"
    Method     string  // "GET", "POST", ""
    Path       string
    AuthScheme string  // "bearer", "api_key", "oauth", "none"
}
```

---

## Edge

```go
type Edge struct {
    // Identity
    ID    string
    Kind  EdgeKind

    // Endpoints
    FromID string  // source node ID
    ToID   string  // destination node ID

    // What flows across this edge
    DataShape DataShape

    // Does the shape match contracts on both ends
    // Stored as a property, not computed on demand
    Compatibility EdgeCompatibility

    // Execution history
    LastObservedAt   time.Time
    ObservationCount int

    // Source
    Source EdgeSource
}
```

### EdgeKind

```go
type EdgeKind string

const (
    EdgeCalls     EdgeKind = "calls"      // function calls function
    EdgeReads     EdgeKind = "reads"      // transformation reads from storage
    EdgeWrites    EdgeKind = "writes"     // transformation writes to storage
    EdgeTriggers  EdgeKind = "triggers"   // schedule/event triggers transformation
    EdgeRunsOn    EdgeKind = "runs_on"    // transformation runs on compute
    EdgeVia       EdgeKind = "via"        // transformation uses connector
    EdgePartOf    EdgeKind = "part_of"    // node belongs to environment
    EdgeDependsOn EdgeKind = "depends_on" // dataset depends on upstream dataset
)
```

### DataShape

```go
type DataShape struct {
    Fields       []Field
    Format       string  // "arrow", "json", "protobuf", "native", ""
    Completeness string  // "full", "partial", "inferred"
}
```

### EdgeCompatibility

```go
type EdgeCompatibility string

const (
    CompatibilityUnknown  EdgeCompatibility = "unknown"   // not yet checked
    CompatibilityFull     EdgeCompatibility = "full"      // shapes match exactly
    CompatibilityPartial  EdgeCompatibility = "partial"   // subset flows, rest ignored
    CompatibilityBreaking EdgeCompatibility = "breaking"  // shape mismatch detected
)
```

When a contract changes, compatibility is updated on all edges touching that node. The agent queries "show me all breaking edges" in a single traversal.

### EdgeSource

```go
type EdgeSource string

const (
    EdgeSourceParsed      EdgeSource = "parsed"       // extracted from source code
    EdgeSourceOpenLineage EdgeSource = "openlineage"  // received from external tool
    EdgeSourceDeclared    EdgeSource = "declared"     // explicitly defined
    EdgeSourceDiscovered  EdgeSource = "discovered"   // inferred from execution traces
)
```

---

## Contract

The agreement. Describes what data a node takes in and puts out. Optional — a node can exist without a contract. Its absence is surfaced as a `ContractGap`.

Contracts are **versioned with valid time**. A node can have many contract versions over its lifetime. Only one is active at any moment (`ValidUntil == nil`). Every `ExecutionTrace` pins the exact contract version that was active when it ran — so you can always answer "what did this node promise when this trace executed?" A contract change has a blast radius: it may break downstream edges, which may require downstream nodes to update their own contracts.

```go
type Contract struct {
    ID           string
    NodeID       string
    Version      string     // "1", "2", "1.1" — increments on each change
    Inputs       []Field
    Outputs      []Field
    Completeness string     // "full", "partial", "inferred"
    Guarantees   []Guarantee

    // Bitemporal valid time
    ValidFrom    time.Time
    ValidUntil   *time.Time // nil = currently active

    // Change tracking
    ChangeSummary string
    IsBreaking    bool
    ChangedBy     string    // "parsed", "openlineage", "agent", "human"

    CreatedAt     time.Time
}

type Field struct {
    Name     string
    Type     string    // language-agnostic type string
    Nullable bool
    Expanded []Field   // one-hop struct/object field expansion
}
```

---

## Guarantee

The test of the agreement. A specific promise about what must be true — before execution, after execution, or always. Guarantees are either declared by a developer (a hypothesis) or discovered from execution evidence (a conclusion).

```go
type Guarantee struct {
    // When does this guarantee apply
    When     GuaranteeWhen

    // What category and specific kind
    Category GuaranteeCategory
    Kind     GuaranteeKind

    // Which field this applies to — empty means the whole node
    Field    string

    // Hypothesis or evidence
    Source   GuaranteeSource

    // Where this guarantee came from
    Origin   GuaranteeOrigin

    // Evidence accumulation
    Confidence   float64  // 0.0 - 1.0, grows with supporting observations
    Observations int      // total traces evaluated against this guarantee
    Violations   int      // traces where this guarantee failed

    // Parameters for kinds that need them
    // e.g. Range:        {"min": "0", "max": "1000"}
    // e.g. Format:       {"pattern": "^[A-Z0-9]{8}$"}
    // e.g. Freshness:    {"max_age_minutes": "60"}
    // e.g. Completeness: {"max_null_rate": "0.01"}
    Params   map[string]string

    DeclaredAt time.Time
    LastSeenAt time.Time

    // Bitemporal valid time
    ValidFrom  time.Time
    ValidUntil *time.Time  // nil = currently active

    // Change tracking
    ChangeSummary string
    ChangedBy     string   // "parsed", "openlineage", "agent", "human"
}
```

### GuaranteeWhen

```go
type GuaranteeWhen string

const (
    WhenPrecondition  GuaranteeWhen = "precondition"  // must be true before execution — caller's responsibility
    WhenPostcondition GuaranteeWhen = "postcondition" // must be true after execution — node's responsibility
    WhenInvariant     GuaranteeWhen = "invariant"     // must always be true
)
```

This dimension determines fault ownership during root cause analysis:
- A precondition violation means the upstream caller is at fault
- A postcondition violation means the node itself is at fault
- An invariant violation means the system is in an invalid state

### GuaranteeSource

```go
type GuaranteeSource string

const (
    GuaranteeSourceDeclared   GuaranteeSource = "declared"   // stated by a developer — a hypothesis
    GuaranteeSourceDiscovered GuaranteeSource = "discovered" // promoted from execution evidence — a conclusion
)
```

Declared guarantees start with `Confidence: 1.0` by assertion. Discovered guarantees start low and accumulate evidence. A declared guarantee that keeps failing is more alarming than an unconfirmed discovered one — the developer said it would hold and it isn't.

### GuaranteeOrigin

Where the guarantee came from. Existing tests are a primary source — a unit test asserting a precondition is a guarantee. A dbt `not_null` test is a guarantee. The system captures what already exists; writing new tests from contracts is a future capability.

```go
type GuaranteeOrigin struct {
    Kind       GuaranteeOriginKind
    SourceRef  string  // file path, test name, dbt test name, suite name
    LineNumber int     // if extracted from source code
}

type GuaranteeOriginKind string

const (
    OriginDeclared           GuaranteeOriginKind = "declared"           // written in contract config
    OriginExtractedTest      GuaranteeOriginKind = "extracted_test"     // parsed from a unit test
    OriginExtractedAssertion GuaranteeOriginKind = "extracted_assertion" // parsed from assert statement
    OriginDbtTest            GuaranteeOriginKind = "dbt_test"           // from dbt schema.yml test
    OriginGreatExpectations  GuaranteeOriginKind = "great_expectations" // from GE suite
    OriginOpenLineage        GuaranteeOriginKind = "openlineage"        // from external tool facet
    OriginDiscovered         GuaranteeOriginKind = "discovered"         // inferred from execution traces
)
```

### GuaranteeCategory and Kind

```go
type GuaranteeCategory string

const (
    CategoryShape       GuaranteeCategory = "shape"        // data structure guarantees
    CategoryBehavioral  GuaranteeCategory = "behavioral"   // execution behavior guarantees
    CategoryOperational GuaranteeCategory = "operational"  // SLA/uptime guarantees
    CategoryQuality     GuaranteeCategory = "quality"      // data quality guarantees
    CategoryGovernance  GuaranteeCategory = "governance"   // compliance/security guarantees
)

type GuaranteeKind string

const (
    // Shape
    GuaranteeNonNull              GuaranteeKind = "non_null"
    GuaranteeUniqueKey            GuaranteeKind = "unique_key"
    GuaranteePositive             GuaranteeKind = "positive"
    GuaranteeRange                GuaranteeKind = "range"
    GuaranteeFormat               GuaranteeKind = "format"
    GuaranteeReferentialIntegrity GuaranteeKind = "referential_integrity"

    // Behavioral
    GuaranteeIdempotent           GuaranteeKind = "idempotent"
    GuaranteeRowCountPreserved    GuaranteeKind = "row_count_preserved"
    GuaranteeOrdered              GuaranteeKind = "ordered"
    GuaranteeNoSideEffects        GuaranteeKind = "no_side_effects"
    GuaranteeCardinality          GuaranteeKind = "cardinality"

    // Operational
    GuaranteeMaxLatency           GuaranteeKind = "max_latency"
    GuaranteeRetryable            GuaranteeKind = "retryable"
    GuaranteeFreshness            GuaranteeKind = "freshness"
    GuaranteeAvailability         GuaranteeKind = "availability"
    GuaranteeRetention            GuaranteeKind = "retention"
    GuaranteeTimeliness           GuaranteeKind = "timeliness"

    // Quality
    GuaranteeCompleteness         GuaranteeKind = "completeness"
    GuaranteeAccuracy             GuaranteeKind = "accuracy"
    GuaranteeSchemaCompatibility  GuaranteeKind = "schema_compatibility"

    // Governance
    GuaranteePII                  GuaranteeKind = "pii"
    GuaranteeSensitivity          GuaranteeKind = "sensitivity"
    GuaranteeAccessControl        GuaranteeKind = "access_control"
)
```

---

## ContractGap

A missing or incomplete contract is not an error — it is information. `ContractGap` surfaces what is absent so the agent can act on it. Gaps are query results, not stored state.

```go
type ContractGap struct {
    NodeID   string
    Kind     ContractGapKind
    Field    string  // empty if the whole contract is missing
    Severity string  // "blocking", "warning", "info"
}

type ContractGapKind string

const (
    GapNoContract        ContractGapKind = "no_contract"         // node has no contract at all
    GapMissingInputType  ContractGapKind = "missing_input_type"  // input field has no type
    GapMissingOutputType ContractGapKind = "missing_output_type" // output field has no type
    GapNoGuarantees      ContractGapKind = "no_guarantees"       // shape known, no promises made
    GapInferredOnly      ContractGapKind = "inferred_only"       // all inferred, nothing declared
)
```

A node with no contract has `GapNoContract`. A Python function with no annotations has `GapMissingInputType` or `GapMissingOutputType` per untyped field. A fully typed function with no guarantees has `GapNoGuarantees` — we know the shape but no promises have been made about it.

The gap queries parallel the existing `untested` query in CodeValent:
- `cvalent query untested` — functions with no test coverage
- `cvalent query ungoverned` — nodes with no contract or incomplete guarantees

---

## ExecutionTrace

The evidence. Records what actually happened when a node ran, what shape data actually had, and whether guarantees held.

```go
type ExecutionTrace struct {
    // Identity
    ID                string
    NodeID            string
    RunID             string     // matches OpenLineage run ID if from external tool

    // Contract pinning — exact contract version active when this trace ran
    // Resolved at write time. Allows full point-in-time contract reconstruction.
    ContractVersionID string     // nil if node had no contract when it ran

    // Timing
    StartedAt   time.Time
    CompletedAt time.Time
    Duration    time.Duration

    // Outcome
    Status TraceStatus

    // What actually flowed — observed, not declared
    InputShape  DataShape
    OutputShape DataShape

    // Contract check results
    Violations []ContractViolation

    // Source
    Source TraceSource

    // Raw context — what the meta-agent reads selectively
    // The agent reads what it needs via selective access, not summaries
    Context TraceContext
}

type TraceStatus string

const (
    TraceStatusSuccess TraceStatus = "success"
    TraceStatusFailure TraceStatus = "failure"
    TraceStatusPartial TraceStatus = "partial"  // completed but with violations
    TraceStatusTimeout TraceStatus = "timeout"
)

type TraceSource string

const (
    TraceSourceOpenLineage TraceSource = "openlineage"  // from Airflow, Spark, dbt, Dagster
    TraceSourceNative      TraceSource = "native"       // from your own pipeline runtime
    TraceSourceTest        TraceSource = "test"         // from CI/test execution
    TraceSourceAgent       TraceSource = "agent"        // agent-observed during task
)

type TraceContext struct {
    Logs           []string
    ErrorMsg       string
    StackTrace     string
    EngineVersion  string
    ConnectorState string
    ExternalRunID  string  // Airflow run_id, Spark job_id, dbt run_id
    ExternalURL    string  // link back to source system UI for deeper context
}
```

---

## ContractViolation

A recorded divergence between what was promised and what was observed. Each violation feeds back into the confidence of the relevant guarantee.

```go
type ContractViolation struct {
    GuaranteeKind GuaranteeKind
    When          GuaranteeWhen
    Field         string
    Expected      string            // what the contract declared
    Observed      string            // what the execution produced
    Severity      ViolationSeverity
    SampleValue   string            // an example of the violating value
}

type ViolationSeverity string

const (
    SeverityBreaking ViolationSeverity = "breaking"  // downstream will fail
    SeverityWarning  ViolationSeverity = "warning"   // downstream may degrade
    SeverityInfo     ViolationSeverity = "info"      // noteworthy, not harmful
)
```

---

## The Feedback Loop

Every `ExecutionTrace` feeds back into the `Contract` of the node it executed:

```
1. For each Guarantee on the node, check the trace output
2. Guarantee held    → increment Observations, confidence moves up
3. Guarantee failed  → increment Observations + Violations,
                       record ContractViolation, confidence moves down
4. Confidence too low → NodeState transitions to "broken"
5. Confidence high on a discovered guarantee
                     → candidate for promotion: experimental → locked
```

This is the hypothesis/evidence loop:

```
Declared guarantee   =  hypothesis
Execution trace      =  the test
Discovered guarantee =  conclusion drawn from sufficient evidence
```

The loop also feeds gap detection. A node that has been running for many traces with no declared contract is a candidate for automated contract proposal — the agent has enough evidence to suggest one.

---

## OpenLineage Compatibility

This schema is a superset of OpenLineage, not a replacement. External tools continue to operate unchanged.

| OpenLineage | Valent |
|---|---|
| `Job` | `Node` (transformation kinds) |
| `Dataset` | `Node` (storage kinds) |
| `Run` | `ExecutionTrace` |
| `Facet` | `Contract`, `Guarantee`, `KindMeta` |
| `namespace` | `EnvironmentID` |
| `name` | `QualifiedName` |

When an OpenLineage event arrives from Airflow, Spark, dbt, or Dagster:
- `Job` → `Node` with `Source: "openlineage"`
- Input/output `Dataset` → `Node` storage nodes
- `Run` → `ExecutionTrace`
- Standard schema facets → `Contract.Inputs` and `Contract.Outputs`
- Valent guarantees added as custom `valent:` namespace facets

The `valent:contract` facet is ignored by OpenLineage-compatible tools and processed by the Valent system. Teams using existing tools get the contract layer grafted on without changing anything. Teams building natively on Valent get full contracts from day one.

---

## Node Identity

Every node is uniquely identified by the composite key:

```
(EnvironmentID, Kind, QualifiedName)
```

The `ID` field is a UUID derived deterministically from this composite — same inputs always produce the same ID. This makes the graph rebuildable from source without losing trace history, because node IDs remain stable across rebuilds.

---

## Validation Rules

```
Every node:
  - Must have ID, Kind, Name, QualifiedName, EnvironmentID
  - Contract is optional — nil is valid, surfaces as ContractGap
  - FunctionMeta required if Kind == "function"
  - PipelineMeta required if Kind == "pipeline_step"
  - StorageMeta required if Kind == "table", "stream", or "bucket"
  - EndpointMeta required if Kind == "endpoint"
  - Credentials must never contain credential values — CredentialRef only

Every edge:
  - Must have ID, Kind, FromID, ToID
  - FromID and ToID must reference existing nodes
  - Compatibility must be recomputed when either endpoint contract changes

Every guarantee:
  - Must have When, Category, Kind, Origin
  - Params must be present for kinds that require them:
    range, format, freshness, completeness, cardinality
  - Violations cannot exceed Observations

Every execution trace:
  - Must have ID, NodeID, StartedAt, Status
  - Violations must reference valid GuaranteeKind values
  - InputShape and OutputShape should be populated even on failure
    (captures what was received, not just that it failed)
```
