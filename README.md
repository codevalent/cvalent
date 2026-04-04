# CodeValent

Local CLI that parses code with tree-sitter, extracts function contracts, resolves cross-file relationships, builds a data flow graph in GoraphDB, and makes it queryable via CLI commands and an MCP server.

Code never leaves your machine.

## Quick Start

```bash
# Build
make build

# Initialize (auto-detects languages)
cvalent init

# Build the code graph
cvalent build

# Query
cvalent query callers ProcessOrder
cvalent query impact validate --depth 3
cvalent query untested
cvalent query coupling

# MCP server (for AI agents)
cvalent serve --mcp
```

## Supported Languages

| Language   | Contract Depth |
|------------|---------------|
| Go         | Full          |
| Java       | Full          |
| TypeScript | Full (~95%)   |
| Python     | Full (annotated) / Inferred (unannotated) |

## Query Commands

| Command | Description |
|---------|-------------|
| `callers <fn>` | Who calls this function? |
| `contract <fn>` | What does it expect/return? |
| `impact <fn>` | Blast radius of changing this function |
| `breaks <fn>` | Callers with stale data shapes |
| `entry-points` | Functions with no incoming calls |
| `exports <module>` | Public API of a module |
| `domains` | Module groupings with function counts |
| `domain <name>` | Functions within a module |
| `coupling` | Cross-module dependency density |
| `untested` | Functions with no test coverage |
| `test-coverage <fn>` | Tests that exercise a function |

## License

MIT
