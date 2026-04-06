# CodeValent

Local code graph for AI agents and developers. Parse, resolve, query — code never leaves your machine.

## What It Does

CodeValent uses tree-sitter to parse source code, extract function contracts (parameters, return types, error shapes), and resolve cross-file call relationships. It stores everything in a GoraphDB graph database local to your project, then exposes the full graph through CLI commands and an MCP server for AI agents.

## Quick Start

```bash
cvalent init        # auto-detect languages, create .cvalent/
cvalent build       # parse + resolve + build graph
cvalent query impact ProcessOrder --depth 3
```

## Install

**Homebrew**

```bash
brew install codevalent/tap/cvalent
```

**Binary download**

Pre-built binaries for Linux and macOS: [GitHub Releases](https://github.com/codevalent/cvalent/releases)

**From source**

```bash
go install github.com/codevalent/cvalent/cmd/cvalent@latest
```

Requires Go 1.25+ and a C compiler (tree-sitter uses cgo).

## Supported Languages

| Language   | Contract Depth                             |
|------------|--------------------------------------------|
| Go         | Full                                       |
| Java       | Full                                       |
| TypeScript | Full (~95%)                                |
| Python     | Full (annotated) / Inferred (unannotated)  |

## CLI Commands

### Project

| Command              | Description                              |
|----------------------|------------------------------------------|
| `cvalent init`       | Auto-detect languages, create `.cvalent/` config |
| `cvalent build`      | Parse, resolve, and build the code graph |
| `cvalent serve --mcp`| Start MCP server over stdio              |

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
