# How It Works

CodeValent builds a queryable code graph in four stages: parse, resolve, build, query. Everything runs locally.

## Pipeline

```
Source files
     |
     v
[1] Parse (tree-sitter) --> Function nodes + contracts
     |
     v
[2] Resolve (cross-file) --> Call edges, import edges, test edges
     |
     v
[3] Build (SQLite) --> Persistent store
     |
     v
[4] Query (CLI / MCP) --> Answers
```

### 1. Parse

Each source file is parsed using [tree-sitter](https://tree-sitter.github.io/tree-sitter/) via the `go-tree-sitter` bindings. Tree-sitter produces a concrete syntax tree (CST) -- a full structural representation of the source, not just an approximation.

For each function or method found, CodeValent extracts a **FunctionNode** containing:

- **Identity**: name, qualified name, file path, package, start/end lines
- **Kind**: function or method (methods include receiver type)
- **Contract**: parameter names and types, return types, nullability, variadic markers
- **Completeness**: `full` (all types known), `partial` (some types missing), or `inferred` (names only, no type annotations)
- **Tag**: `application` or `test` (determined by file path and naming conventions like `_test.go`, `test_*.py`, `*Test.java`)
- **Export status**: whether the function is part of the module's public API

For struct/class parameters, one-hop field expansion is performed -- the graph knows not just that a function takes an `OrderRequest`, but what fields `OrderRequest` contains.

### 2. Resolve

The resolver connects function nodes across files:

- **Symbol index**: every exported function and class, keyed by module path
- **Import resolution**: traces import statements to their corresponding symbols
- **Call resolution**: matches call sites to function definitions, producing call edges
- **Test edges**: calls from test functions to application functions are tagged, enabling coverage queries
- **Data flow**: each call edge carries the data shape that flows across it -- not just "A calls B" but "A passes `{id: string, amount: float}` to B"

### 3. Build

The resolved graph (nodes + edges) is written to an embedded SQLite store. The database file lives at `.cvalent/store.db`.

Every `cvalent build` performs a full rebuild -- no incremental logic. This keeps the implementation simple and the graph always consistent. Target performance is sub-5 seconds for typical repositories.

### 4. Query

The CLI and MCP server share a single query engine. The engine runs graph traversals (BFS for impact analysis, direct lookups for contracts, filtered scans for untested functions, etc.) and returns structured results.

- **CLI** formats results for the terminal
- **MCP server** returns JSON over stdio, with pagination support (limit/offset)

Both interfaces call the same `query.*` functions -- the MCP server is not a separate implementation, just a different transport.

## What the graph contains

- **Nodes**: functions and methods with full contract metadata
- **Call edges**: function A calls function B, with data shape annotations
- **Import edges**: module-level dependency relationships
- **Test edges**: which test functions exercise which application functions

## Storage

SQLite is embedded in the binary -- no external database server needed. The `.cvalent/store.db` file is a local cache, always rebuildable from source. It is gitignored by default.
