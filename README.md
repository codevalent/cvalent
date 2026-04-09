# cvalent

Local code graph for AI agents and developers. Parse, resolve, query — code never leaves your machine.

## What It Does

cvalent uses tree-sitter to parse source code, extract function contracts (parameters, return types, error shapes), and resolve cross-file call relationships. It stores everything in an embedded SQLite database local to your project, then exposes the full graph through CLI commands and an MCP server for AI agents.

Every function in the graph carries a canonical identity (Model B) based on its distribution, module path, and name. Two repos importing the same published module produce identical UUIDs for shared functions, so that when you move to the hosted version (Rung 1+) those graphs merge without re-minting IDs.

## Quick Start

```bash
cvalent init        # auto-detect languages, create .cvalent/
cvalent build       # parse + resolve + build store
cvalent query impact ProcessOrder --depth 3
```

### Upgrading from pre-0.2.0

If you have a `.cvalent/graph.db` from an older version:

```bash
cvalent migrate-store   # one-shot migration, graph.db -> store.db
```

## Install

**Binary download**

Pre-built binaries for Linux and macOS (amd64 + arm64): [GitHub Releases](https://github.com/codevalent/cvalent/releases)

No C toolchain required — cvalent ships as a single static binary.

**From source**

```bash
go install github.com/codevalent/cvalent/cmd/cvalent@latest
```

Requires Go 1.25+.

## Supported Languages

| Language   | Distribution Resolution         | Contract Depth                            |
|------------|---------------------------------|-------------------------------------------|
| Go         | `go.mod` module path            | Full                                      |
| Java       | `pom.xml` / `build.gradle`      | Full                                      |
| TypeScript | `package.json` name field       | Full (~95%)                               |
| Python     | `pyproject.toml` / `setup.cfg`  | Full (annotated) / Inferred (unannotated) |

## CLI Commands

### Project

| Command               | Description                              |
|-----------------------|------------------------------------------|
| `cvalent init`        | Auto-detect languages, create `.cvalent/` config |
| `cvalent build`       | Parse, resolve, and build the store      |
| `cvalent serve --mcp` | Start MCP server over stdio              |
| `cvalent migrate-store` | Convert legacy graph.db to store.db    |

### Query

| Command                    | Description                                  |
|----------------------------|----------------------------------------------|
| `query callers <fn>`       | Who calls this function?                     |
| `query contract <fn>`      | What does it expect and return?              |
| `query impact <fn>`        | Blast radius of changing this function       |
| `query breaks <fn>`        | Callers with stale data shapes               |
| `query entry-points`       | Functions with no incoming calls             |
| `query exports <module>`   | Public API of a module                       |
| `query domains`            | Module groupings with function counts        |
| `query domain <name>`      | Functions within a module                    |
| `query coupling`           | Cross-module dependency density              |
| `query untested`           | Functions with no test coverage              |
| `query test-coverage <fn>` | Tests that exercise a function               |

## MCP Server

```bash
cvalent serve --mcp
```

Starts a stdio-based MCP server exposing 13 tools that mirror the CLI query surface. AI agents and editors can connect over standard MCP protocol. See [docs/mcp-setup.md](docs/mcp-setup.md) for editor configuration.

The seven tools that touch cross-repo boundaries (`callers`, `impact`, `breaks`, `test_coverage`, `subgraph`, `untested`, `entry_points`) include a `boundaries` array and `boundary_signal` field so agents can detect when the hosted store would unlock additional resolution.

## Platform Support

| OS      | amd64 | arm64 |
|---------|-------|-------|
| Linux   | Yes   | Yes   |
| macOS   | Yes   | Yes   |
| Windows | --    | --    |

Windows support is planned for a future release.

## License

[MIT](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
