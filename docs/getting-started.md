# Getting Started

## Prerequisites

- **Go 1.25+** ([install](https://go.dev/doc/install))
- **C compiler** (gcc, clang, or Xcode command-line tools) -- tree-sitter uses cgo

Verify:

```bash
go version    # go1.25.0 or later
cc --version  # any working C compiler
```

## Install

**Homebrew** (macOS/Linux)

```bash
brew install codevalent/tap/cvalent
```

**Pre-built binary**

Download from [GitHub Releases](https://github.com/codevalent/cvalent/releases) for your OS and architecture. Add to your PATH.

**From source**

```bash
go install github.com/codevalent/cvalent/cmd/cvalent@latest
```

## First Project

Navigate to any Go, Java, TypeScript, or Python project:

```bash
cd ~/your-project
```

### 1. Initialize

```bash
cvalent init
```

This creates a `.cvalent/` directory with auto-detected language configuration. You will see output like:

```
Initialized .cvalent/ with languages: [go typescript]
```

### 2. Build the graph

```bash
cvalent build
```

Parses all source files with tree-sitter, resolves cross-file relationships, and stores the graph in `.cvalent/graph.db`. A full rebuild runs every time -- there is no incremental mode yet.

### 3. Query

Find out what calls a function:

```bash
cvalent query callers ProcessOrder
```

See the blast radius of a change:

```bash
cvalent query impact ProcessOrder --depth 3
```

Find untested functions:

```bash
cvalent query untested
```

### 4. MCP server (optional)

Start the MCP server so AI agents can query the graph:

```bash
cvalent serve --mcp
```

See [mcp-setup.md](mcp-setup.md) for editor configuration.

## What just happened

1. `init` scanned your project for `.go`, `.java`, `.ts`, `.tsx`, `.py` files and wrote a config
2. `build` parsed every detected file, extracted function contracts (params, returns, completeness), resolved imports and call edges across files, and stored everything in a local GoraphDB database
3. `query` ran graph queries against that database -- no network, no cloud, everything local
