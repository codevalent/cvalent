# Contributing

## Build from Source

Requirements: Go 1.25+, C compiler (tree-sitter uses cgo).

```bash
git clone https://github.com/codevalent/cvalent.git
cd cvalent
make build
```

This produces a `cvalent` binary in the project root.

## Run Tests

```bash
make test          # full test suite, verbose
make test-short    # skip long-running tests
```

Tests use `go test ./...` with `-count=1` (no caching).

## Code Style

- Run `gofmt` before committing. No external linters are configured yet.
- Minimal comments -- only where logic is not self-evident.
- No emojis in code or documentation.

## Project Structure

```
cmd/
  cvalent/
    main.go          # CLI entry point, cobra command wiring

internal/
  build/             # orchestrates parse + resolve + graph construction
  config/            # .cvalent/ directory, config.json, language detection
  graph/             # GoraphDB wrapper (open, close, node/edge operations)
  mcp/               # MCP server (stdio JSON-RPC, tool definitions)
  parser/            # tree-sitter parsing, FunctionNode extraction per language
  query/             # query engine (callers, impact, coupling, etc.)
  resolver/          # cross-file symbol resolution, import/call edge building
  walker/            # directory traversal, .gitignore and exclude handling
```

Key design decisions:

- **Single query engine**: CLI and MCP server both call the same `query.*` functions. CLI formats for terminal, MCP formats for JSON.
- **`internal/` only**: all packages are internal. The public API is the CLI binary and MCP protocol.
- **No external dependencies for core logic**: tree-sitter, GoraphDB, and cobra are the only significant dependencies.

## Making Changes

1. Fork the repository
2. Create a branch from `main`
3. Make your changes
4. Run `make test` -- all tests must pass
5. Run `gofmt -w .` to format
6. Open a pull request against `main`

Keep PRs focused. One logical change per PR.

## Adding Language Support

Language parsers implement the `LanguageParser` interface in `internal/parser/types.go`:

```go
type LanguageParser interface {
    Parse(filepath string, source []byte) ([]FunctionNode, error)
    Language() string
}
```

To add a new language:

1. Add a new file in `internal/parser/` (e.g., `ruby.go`)
2. Implement `LanguageParser` using the appropriate tree-sitter grammar
3. Register the language in the parser registry
4. Add the file extension mapping in `internal/config/config.go`
5. Add tests in `testdata/` with representative source files
