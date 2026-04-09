# Configuration

## The `.cvalent/` Directory

Running `cvalent init` creates a `.cvalent/` directory in your project root:

```
.cvalent/
  config.json    # project configuration
  store.db       # SQLite store (gitignored)
  .gitignore     # ignores store.db
```

The `store.db` file is automatically gitignored. It is a cache -- always rebuildable from source via `cvalent build`.

## config.json

```json
{
  "version": 1,
  "languages": ["go", "typescript"],
  "exclude": ["vendor", "node_modules", ".git", "__pycache__", "dist", "build"]
}
```

### Fields

| Field       | Type       | Description                                      |
|-------------|------------|--------------------------------------------------|
| `version`   | `int`      | Config schema version. Currently `1`.            |
| `languages` | `string[]` | Languages to parse. Auto-detected on `init`.     |
| `exclude`   | `string[]` | Directory names to skip during parsing/walking.  |

### `version`

Always `1` for now. Will increment if the config schema changes in future releases.

### `languages`

Valid values: `go`, `java`, `typescript`, `python`.

Auto-detected by `cvalent init` based on file extensions found in the project:

| Extension     | Language     |
|---------------|--------------|
| `.go`         | `go`         |
| `.java`       | `java`       |
| `.ts`, `.tsx` | `typescript` |
| `.py`         | `python`     |

You can manually add or remove languages after init. Only listed languages will be parsed during `cvalent build`.

### `exclude`

Directory names (not glob patterns) that the walker skips entirely. Defaults:

- `vendor`
- `node_modules`
- `.git`
- `__pycache__`
- `dist`
- `build`

To add project-specific excludes, edit the array:

```json
{
  "exclude": ["vendor", "node_modules", ".git", "__pycache__", "dist", "build", "generated", "third_party"]
}
```

## Auto-Detection

`cvalent init` walks the project tree (skipping `.git`, `vendor`, `node_modules`, and `.cvalent`) and checks file extensions. If it finds at least one file matching a supported extension, that language is added to the config.

The walk respects the same skip rules as the build walker, so detection results match what `cvalent build` will actually parse.
