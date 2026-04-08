# Identity Specification

This document is the authoritative reference for the canonical qualified-name
form used by `internal/model/identity.Canonicalize` and the UUID derivation
performed by `internal/model/identity.NewID`.

Every parser, the migrator, the parity harness, and every test in cvalent
must construct identities exclusively through these two functions. There is
no other supported way to mint a node identity.

## UUID derivation

Node UUIDs are produced as `uuid.NewSHA1(NamespaceCvalent, payload)` —
i.e. UUIDv5 with a fixed namespace constant defined in
`internal/model/identity.go`:

```
NamespaceCvalent = uuid.MustParse("0a45c2e0-2c5b-4d4f-8a9b-cvalent00001")
```

The payload is the canonical string:

```
<environment> "|" <kind> "|" <qualified_name>
```

- `environment` — at Rung 0 always the literal `"local"`. Reserved for the
  hosted store (Rung 1+) which will introduce real environment names.
- `kind` — `"function"` or `"method"`. (Other kinds are reserved for later
  rungs and rejected at Rung 0.)
- `qualified_name` — the output of `Canonicalize(IdentityParts)` exactly,
  without any additional whitespace or normalization.

## IdentityParts

`IdentityParts` is the only input shape accepted by `Canonicalize`. The
canonical qualified-name string is stored on `IdentityParts` in an
**unexported** field `qualifiedName`. Callers cannot construct or mutate
that field — it is only set by `Canonicalize` itself, and only read by
`NewID`. This is what makes `Canonicalize` the single source of truth.

```go
type IdentityParts struct {
    Distribution string   // distribution name, e.g. "github.com/foo/bar"
    ModulePath   string   // dot-separated package/module path
    Container    string   // empty for free functions; receiver type for methods
    Name         string   // function/method name
    Params       []string // parameter type list, used for overload disambiguation
    PointerRecv  bool     // true for Go pointer receivers
    qualifiedName string  // populated by Canonicalize, read by NewID
}
```

## Canonical qualified-name form

```
<distribution>/<module-path>.<container?>.<name><sigHash?>
```

Construction rules — applied in order, by `Canonicalize`:

1. **Unicode normalization.** Every input field is NFC-normalized
   (`golang.org/x/text/unicode/norm`) before any further processing. This
   ensures that visually identical names from different source files do not
   mint different identities just because of normalization form.

2. **Distribution.** Inserted verbatim. The distresolver is responsible for
   producing one of:
   - a real distribution string (e.g. `github.com/foo/bar`,
     `org.apache.commons:commons-lang3`, `python-dotenv`,
     `@scope/package-name`),
   - a `repo:<account>/<repo>` fallback when no manifest was found,
   - a `repo:<basename>` fallback when there is no git remote.

3. **Module path.** Joined to the distribution with a single forward slash
   (`/`). Empty module paths are allowed and produce
   `<distribution>/.<container?>.<name><sigHash?>`.

4. **Container.** Optional. For methods this is the receiver type:
   - **Pointer receivers** are written as `(*T)` literally — for example,
     `(*OrderRequest)`. The `PointerRecv` field on `IdentityParts` controls
     this and must not be inferred from the `Container` string.
   - **Value receivers** are written as `T` (no parentheses).
   - **Generic types** have their type-parameter list stripped:
     `Box[T]` and `Box[int]` both produce container `Box`.

5. **Generics — type parameters.** All type-parameter lists on the function
   itself are stripped before canonicalization. Two instantiations of the
   same generic produce the same UUID. This is intentional: cvalent's
   identity surface is defined at the source level, not at the
   monomorphized level.

6. **Overload disambiguation — `sigHash`.** For Java and TypeScript
   (languages where the same name can have multiple type signatures in the
   same scope), the parameter list is hashed and appended as
   `#<sigHash>` where `sigHash` is the **first 8 hex characters** of the
   SHA-256 of the parameter type list joined by `,` (no spaces). For Go and
   Python (which do not allow overloading), the `sigHash` suffix is omitted
   even when `Params` is set.

7. **Anonymous and closure functions.** Not nodes at Rung 0. `Canonicalize`
   returns an empty string and `NewID` returns the zero UUID with an error
   if asked to mint such an identity. Parsers must skip them rather than
   pass them through.

## Property invariants

The following invariants are tested in `internal/model/identity_test.go`
and must remain true:

- **Determinism.** `NewID(e, k, q) == NewID(e, k, q)` for any inputs.
- **Pointer vs value receivers** produce different UUIDs even when every
  other field matches.
- **Generic instantiations** of the same generic function produce the same
  UUID. (`Box[int].Get` and `Box[string].Get` collapse to one identity.)
- **Java/TS overloads** with different parameter lists produce different
  UUIDs (the `sigHash` suffix differs).
- **Anonymous function inputs** are rejected.
- **Unicode equivalence.** NFC and NFD spellings of the same identifier
  produce the same UUID.

## What this spec does NOT cover

Future rungs will add real `environment` values, additional node `kind`s
(pipeline_step, storage, endpoint), and a richer container model. Those
extensions will land as additions to `IdentityParts` and rules in this
document — never as parser-side string formatting.
