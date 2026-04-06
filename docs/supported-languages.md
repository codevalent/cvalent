# Supported Languages

## Overview

| Language   | File Extensions | Contract Depth                        | Status |
|------------|----------------|---------------------------------------|--------|
| Go         | `.go`          | Full (types in syntax)                | Stable |
| Java       | `.java`        | Full (types in syntax)                | Stable |
| TypeScript | `.ts`, `.tsx`   | Full (~95%, some edge cases)          | Stable |
| Python     | `.py`          | Full (annotated) / Inferred (unannotated) | Stable |

## Contract Depth

Contract depth describes how much type information CodeValent can extract from function signatures.

- **Full**: all parameter types and return types are known from the source. Go, Java, and TypeScript enforce types in syntax, so contracts are always complete.
- **Inferred**: parameter names are known but types are not. This happens with unannotated Python functions. CodeValent still extracts the function node and its call edges -- it just marks the contract as `inferred` rather than `full`.

The completeness level (`full`, `partial`, `inferred`) is stored on every function node and queryable via `cvalent query contract`.

## Per-Language Details

### Go

- Functions and methods (with receiver type) are extracted
- All parameter and return types are resolved from syntax
- Test detection: `_test.go` files, functions matching `Test*`, `Benchmark*`, `Example*`
- Exported status determined by capitalization
- Struct field expansion for parameter types

### Java

- Methods, constructors, and static methods are extracted
- Full type information from declarations
- Test detection: `*Test.java` files, `@Test` annotation, methods matching `test*`
- Visibility modifiers determine export status
- Class field expansion for parameter types

### TypeScript

- Functions, methods, and arrow functions are extracted
- Type annotations and interface types are resolved
- TSX files (`.tsx`) are supported alongside `.ts`
- Test detection: `*.test.ts`, `*.spec.ts`, functions matching `test*`, `it()`, `describe()` blocks
- `export` keyword determines export status
- Covers approximately 95% of TypeScript patterns; some advanced generics and mapped types may produce partial contracts

### Python

- Functions, methods, and class methods are extracted
- Type annotations (PEP 484) are used when present; without them, contracts are marked `inferred`
- Test detection: `test_*.py` files, functions matching `test_*`
- All top-level functions are treated as exported (Python has no access modifiers)
- Dataclass and TypedDict field expansion for annotated parameter types

## Known Limitations

- **No incremental parsing**: `cvalent build` re-parses all files every time
- **Dynamic dispatch**: indirect calls (function pointers, reflection, decorators that transform signatures) are not resolved
- **Generics**: complex generic type resolution may produce partial contracts in TypeScript and Java
- **Python without annotations**: contracts are inferred from names only -- no type information flows through call edges
- **Unsupported languages**: files in unsupported languages are skipped with a message indicating which languages were detected but not supported (e.g., "Skipped N .rb files")
