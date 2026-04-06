# Contributing to CodeValent

Thanks for your interest in contributing to CodeValent.

## Build from source

### Prerequisites

- Go 1.25+
- C compiler (required for tree-sitter native parsers)
- Make

### Steps

```bash
git clone https://github.com/codevalent/cvalent.git
cd cvalent
make build
make test
```

The built binary lands in the repo root as `cvalent`.

## Code style

- Run `gofmt` on all Go files before committing.
- Run `go vet ./...` and fix any warnings.
- Keep it simple. Prefer clear code over clever code.

## PR process

1. Fork the repo and create a feature branch from `main`.
2. Make your changes with tests.
3. Run `make test` and confirm everything passes.
4. Open a pull request against `main`.

Keep PRs focused on a single change. If you're fixing a bug and want to refactor nearby code, split those into separate PRs.

## Architecture overview

```
cmd/cvalent/         CLI entry point
internal/
  build/             Build graph construction
  config/            Configuration loading
  graph/             Graph data structures
  mcp/               MCP server implementation
  parser/*/          Language-specific tree-sitter parsers
  query/             Graph query engine
  resolver/          Symbol resolution
  walker/            File tree walking
```

See [docs/contributing.md](docs/contributing.md) for more detail on the internals.

## Reporting issues

- **Bugs**: Use the [bug report template](https://github.com/codevalent/cvalent/issues/new?template=bug_report.md)
- **Features**: Use the [feature request template](https://github.com/codevalent/cvalent/issues/new?template=feature_request.md)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
