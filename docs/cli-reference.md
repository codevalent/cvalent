# CLI Reference

All commands are run as `cvalent <command>`.

## Project Commands

### `init`

Initialize a `.cvalent/` directory with auto-detected configuration.

```bash
cvalent init
```

Walks the project tree, detects languages by file extension, writes `.cvalent/config.json` and `.cvalent/.gitignore`. Safe to re-run -- overwrites existing config.

### `build [path]`

Parse source files, resolve cross-file relationships, and build the code graph.

```bash
cvalent build           # build entire project
cvalent build src/      # build only files under src/
```

Performs a full rebuild every time. The graph is stored at `.cvalent/graph.db`.

### `parse <file>`

Parse a single file and print extracted functions. Useful for debugging parser output.

```bash
cvalent parse src/main.go
```

### `serve`

Start the MCP server.

```bash
cvalent serve --mcp
```

**Flags:**

| Flag    | Description                         |
|---------|-------------------------------------|
| `--mcp` | Required. Start MCP server on stdio |

The `--mcp` flag is required -- running `cvalent serve` without it returns an error. The server exposes 13 tools (all 11 query commands plus `graph_summary` and `subgraph`) over JSON-RPC on stdin/stdout.

Requires a built graph. Run `cvalent build` first.

---

## Query Commands

All query commands require a built graph (`cvalent build`). Usage: `cvalent query <subcommand>`.

### `query callers <function>`

List functions that directly call the specified function.

```bash
cvalent query callers ProcessOrder
```

### `query contract <function>`

Show the contract (parameters, return types, completeness) of a function.

```bash
cvalent query contract ProcessOrder
```

Output includes qualified name, file location, contract shape, and completeness level (full, partial, or inferred).

### `query impact <function>`

Trace the blast radius of changing a function via BFS traversal of downstream callers.

```bash
cvalent query impact ProcessOrder
cvalent query impact ProcessOrder --depth 5
```

**Flags:**

| Flag      | Default | Description                |
|-----------|---------|----------------------------|
| `--depth` | 3       | Maximum traversal depth    |

### `query breaks <function>`

Show callers whose argument shapes do not match the function's current contract.

```bash
cvalent query breaks ProcessOrder
```

### `query entry-points`

List all functions with no incoming call edges -- top-level entry points, main functions, HTTP handlers.

```bash
cvalent query entry-points
```

### `query exports <module>`

List the public (exported) functions of a module.

```bash
cvalent query exports internal/parser
```

### `query domains`

List all directory-based module groupings with function counts.

```bash
cvalent query domains
```

### `query domain <name>`

Show all functions and internal call edges within a single module.

```bash
cvalent query domain internal/parser
```

### `query coupling`

Show cross-module dependency density. Lists module pairs with edge counts, useful for identifying tight coupling.

```bash
cvalent query coupling
```

### `query untested`

List application functions that have no incoming test edges.

```bash
cvalent query untested
```

### `query test-coverage <function>`

List the test functions that exercise a given function.

```bash
cvalent query test-coverage ProcessOrder
```

---

## Global Flags

| Flag        | Description          |
|-------------|----------------------|
| `--version` | Print version        |
| `--help`    | Help for any command |
