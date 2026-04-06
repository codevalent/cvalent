# MCP Setup

Configure your editor to use the cvalent MCP server for code-graph queries.

## Prerequisites

1. **`cvalent` installed** and available on your `PATH` (or note the absolute path to the binary).
2. **`.cvalent/` initialized** in the project root -- run `cvalent init` if you haven't already.
3. **Graph built** -- run `cvalent build` so the server has data to query.

---

## Editor Configurations

### 1. Claude Code

**File:** `.mcp.json` at the project root.

```json
{
  "mcpServers": {
    "cvalent": {
      "command": "cvalent",
      "args": ["serve", "--mcp"]
    }
  }
}
```

> If `cvalent` is not on PATH, replace `"cvalent"` with the absolute path to the binary.

### 2. Cursor

**File:** `.cursor/mcp.json` in the project root.

```json
{
  "mcpServers": {
    "cvalent": {
      "command": "cvalent",
      "args": ["serve", "--mcp"]
    }
  }
}
```

> If `cvalent` is not on PATH, replace `"cvalent"` with the absolute path to the binary.

### 3. VS Code

**File:** `.vscode/settings.json` (workspace) or your user `settings.json`.

Add the following under the `mcp` section:

```json
{
  "mcp": {
    "servers": {
      "cvalent": {
        "type": "stdio",
        "command": "cvalent",
        "args": ["serve", "--mcp"]
      }
    }
  }
}
```

> If `cvalent` is not on PATH, replace `"cvalent"` with the absolute path to the binary.

### 4. Zed

**File:** `.zed/settings.json` (project) or `~/.config/zed/settings.json` (global).

Add the following under the `context_servers` section:

```json
{
  "context_servers": {
    "cvalent": {
      "command": {
        "path": "cvalent",
        "args": ["serve", "--mcp"]
      }
    }
  }
}
```

> If `cvalent` is not on PATH, replace `"cvalent"` in the `path` field with the absolute path to the binary.

---

## Available Tools

The MCP server exposes 13 tools:

| Tool | Description |
|------|-------------|
| `callers` | List all functions that directly call the specified function. |
| `contract` | Return the parameter and return-type contract of a function. |
| `impact` | Trace downstream callers to N levels, showing the blast radius of a change. |
| `breaks` | Detect callers whose argument shape mismatches the function contract. |
| `entry_points` | List all functions with no incoming call edges (top-level entry points). |
| `exports` | List the public (exported) functions of a module. |
| `domains` | List all directory-based module groupings with function counts. |
| `domain` | List functions and internal call edges within a single module. |
| `coupling` | Measure cross-module dependency density across all module pairs. |
| `untested` | List application functions that have no test coverage. |
| `test_coverage` | List the test functions that exercise the specified function. |
| `graph_summary` | Return a compact overview of the pre-built code graph: modules, counts, coverage. |
| `subgraph` | Extract the N-hop neighborhood of a function with contracts and edges. |
