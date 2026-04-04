# Phase 1: The Graph — CodeValent

**Brand**: CodeValent
**Product**: CodeValent
**CLI binary**: `cvalent`
**Layer**: 1 (local, free, open source)
**License**: MIT
**Repo**: Separate GitHub repo, clean room
**Language**: Go, Java, TypeScript, Python

## Naming

CodeValent is the brand — an agreement, a promise. CodeValent is the first product. The naming pattern extends:
- **CodeValent** (`cvalent`) — code contracts, structure, data flow
- **DataValent** (`dvalent`) — pipeline contracts, schema enforcement, lineage (future)

## What ships

`cvalent` — a local binary that parses code with tree-sitter, extracts function contracts, resolves cross-file relationships, builds a data flow graph in GoraphDB, and makes it queryable via CLI and MCP server. Code never leaves the developer's machine.

## Milestones

### M0: Parser Foundation

**Goal**: tree-sitter parses a single file and outputs structured function nodes with contracts.

**What gets built**:
- Project skeleton (Go binary, CLI framework)
- tree-sitter integration via go-tree-sitter
- Language support: Go, Java, Python
- For each function: name, file, lines, parameter names/types, return types
- Contract extraction as first-class output:
  - Declared (type annotations present) vs inferred (names only)
  - Partial contracts explicitly flagged — incompleteness is information
- Test function identification: tag functions as `test` or `application` based on file path + function name patterns (e.g., `_test.go`, `Test*`, `test_*.py`, `test_*`, `*Test.java`, `@Test`)

**Done when**: `cvalent parse src/main.py` prints function nodes with names, lines, parameter names/types, return types, contract completeness flag, and test/application tag. Works for Go, Java, and Python files.

---

### M1: Graph + Resolution

**Goal**: parse an entire project, resolve cross-file relationships, build the data flow graph.

**What gets built**:
- Directory walker (respect .gitignore, configurable excludes)
- Add TypeScript language support (Java added in M0)
- Symbol index: every exported function/class, keyed by module path
- Import resolver: trace imports to the symbol index
- Call resolver: match call sites to function definitions across files
- Contract propagation: for each call edge, what data shape flows across it
- Data flow graph: not just "A calls B" but "A passes {id: string, amount: float} to B"
- Test edges: call edges from test → application code resolve normally, enabling coverage queries

**Done when**: `cvalent build` on a Go/Java/Python/TypeScript project produces a graph with function nodes (tagged test/application), contracts, call edges, import edges, and data flow annotations. Stored in GoraphDB.

---

### M2: CLI Queries

**Goal**: humans and scripts can ask questions about the graph.

**What gets built**:
- `cvalent query callers <function>` — who calls this?
- `cvalent query contract <function>` — what does it expect/return?
- `cvalent query impact <function> [--depth N]` — blast radius via BFS
- `cvalent query breaks <function>` — given a changed contract, which callers break?
- `cvalent query entry-points` — functions with no incoming call edges
- `cvalent query exports <module>` — public API surface of a module
- `cvalent query domains` — list directory-based module groupings with cross-module edge counts
- `cvalent query domain <name>` — all functions, contracts, and edges within a module group
- `cvalent query coupling` — cross-module dependency map, flag heavy cross-dependencies
- `cvalent query untested` — application functions with no incoming test edges
- `cvalent query test-coverage <function>` — which tests exercise this function?

**Done when**: all 11 queries work against local GoraphDB graph.

---

### M3: MCP Server

**Goal**: AI agents can query the graph as MCP tools.

**What gets built**:
- `cvalent serve --mcp` starts MCP server (stdio transport)
- All 11 CLI queries exposed as MCP tools
- Two additional MCP tools (wrappers around CLI query engine):
  - `graph_summary` — compact index (modules, function counts, edge counts, test coverage %)
  - `subgraph` — neighborhood of a function/file (N hops)
- Response format optimized for LLM context: compact, no redundancy
- Context restoration hook: Claude Code config that calls `graph_summary` after compaction events — agent recovers structural context without re-reading source files

**Done when**: Claude Code with MCP configured can answer "what breaks if I change this function?", "what's untested?", and "show me the blast radius" without reading source files.

---

## Architecture

```
Source files
     │
     v
tree-sitter parser ──> AST
     │
     v
Symbol extractor ──> Functions + Contracts (tagged test/application)
     │
     v
Cross-file resolver ──> Call edges + Import edges + Test edges
     │
     v
Data flow resolver ──> What data shapes flow on each edge
     │
     v
Graph builder
     │
     ├──> GoraphDB ──> CLI queries (11 commands)
     │              ──> MCP server ──> AI agents
     │
     (no cloud in Phase 1)
```

Single query engine in Go. CLI and MCP server both call the same functions. CLI formats for terminal. MCP formats for JSON.

## Storage

- GoraphDB, compiled into the binary
- Full parse on every `cvalent build` (no incremental logic in Phase 1)
- Watch mode = re-run build on file save (if fast enough; expected sub-5s on target repos)
- Always rebuildable from source — GoraphDB is a cache, not primary store

## Languages

| Language | Phase | Contract depth |
|---|---|---|
| Go | M0 | Full (types in syntax) |
| Java | M0 | Full (types in syntax) |
| Python | M0 | Full (annotations) or inferred (names only) |
| TypeScript | M1 | Full (types in syntax) |

Unsupported files produce: "Skipped N .rb files (Ruby support coming)" to signal roadmap, not limitation.

## What someone can do when Phase 1 is complete

- Install `cvalent`, run `cvalent init` in any Go/Java/Python/TypeScript repo
- `cvalent build` produces a full data flow graph — every function, contracts, test coverage, edges
- AI agent uses MCP server to answer: "what calls this?", "what's the blast radius?", "what's untested?", "what breaks if I change this?" — without reading source files
- Context restoration hook auto-recovers agent's structural understanding after compaction
- `cvalent query coupling` shows which modules are tightly bound
- All queries available from terminal for scripting and CI integration
