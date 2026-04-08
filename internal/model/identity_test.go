package model

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func mustMint(t *testing.T, parts IdentityParts) (uuid.UUID, IdentityParts) {
	t.Helper()
	id, canon, err := MintFunctionID(EnvironmentLocal, parts)
	if err != nil {
		t.Fatalf("MintFunctionID(%+v) error: %v", parts, err)
	}
	if id == uuid.Nil {
		t.Fatalf("MintFunctionID returned nil UUID for %+v", parts)
	}
	return id, canon
}

func TestNewID_Deterministic(t *testing.T) {
	parts := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         "Frob",
	}
	id1, _ := mustMint(t, parts)
	id2, _ := mustMint(t, parts)
	if id1 != id2 {
		t.Fatalf("NewID not deterministic: %v != %v", id1, id2)
	}
}

func TestCanonicalize_PointerVsValueReceiver(t *testing.T) {
	value := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Container:    "OrderRequest",
		Name:         "Validate",
	}
	pointer := value
	pointer.PointerReceiver = true

	idValue, canonValue := mustMint(t, value)
	idPointer, canonPointer := mustMint(t, pointer)

	if idValue == idPointer {
		t.Fatalf("pointer/value receivers must mint different UUIDs")
	}
	if canonValue.QualifiedName() == canonPointer.QualifiedName() {
		t.Fatalf("pointer/value receivers must produce different qualified names: %q == %q",
			canonValue.QualifiedName(), canonPointer.QualifiedName())
	}
	if want := "github.com/foo/bar/internal/widget.(*OrderRequest).Validate"; canonPointer.QualifiedName() != want {
		t.Fatalf("pointer-receiver canonical form: got %q want %q", canonPointer.QualifiedName(), want)
	}
	if want := "github.com/foo/bar/internal/widget.OrderRequest.Validate"; canonValue.QualifiedName() != want {
		t.Fatalf("value-receiver canonical form: got %q want %q", canonValue.QualifiedName(), want)
	}
}

func TestCanonicalize_GenericInstantiationsCollapse(t *testing.T) {
	intBox := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Container:    "Box[int]",
		Name:         "Get",
	}
	stringBox := intBox
	stringBox.Container = "Box[string]"
	plainBox := intBox
	plainBox.Container = "Box"

	idA, _ := mustMint(t, intBox)
	idB, _ := mustMint(t, stringBox)
	idC, _ := mustMint(t, plainBox)

	if idA != idB || idB != idC {
		t.Fatalf("generic instantiations must collapse: %v %v %v", idA, idB, idC)
	}
}

func TestCanonicalize_GenericFuncTypeParamsStripped(t *testing.T) {
	parts := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         "Map[T any]",
	}
	canon, err := Canonicalize(parts)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if want := "github.com/foo/bar/internal/widget.Map"; canon.QualifiedName() != want {
		t.Fatalf("got %q want %q", canon.QualifiedName(), want)
	}
}

func TestCanonicalize_JavaOverloadDisambiguation(t *testing.T) {
	a := IdentityParts{
		Distribution:     "org.apache.commons:commons-lang3",
		ModulePath:       "org.apache.commons.lang3",
		Container:        "StringUtils",
		Name:             "join",
		Params:           []string{"java.lang.String[]", "java.lang.String"},
		OverloadLanguage: "java",
	}
	b := a
	b.Params = []string{"java.lang.Iterable", "java.lang.String"}

	idA, canonA := mustMint(t, a)
	idB, canonB := mustMint(t, b)

	if idA == idB {
		t.Fatalf("different param lists must produce different UUIDs")
	}
	if canonA.QualifiedName() == canonB.QualifiedName() {
		t.Fatalf("different param lists must produce different canonical names")
	}
}

func TestCanonicalize_TypeScriptOverloadDisambiguation(t *testing.T) {
	a := IdentityParts{
		Distribution:     "@scope/widgets",
		ModulePath:       "src/utils",
		Name:             "format",
		Params:           []string{"string"},
		OverloadLanguage: "typescript",
	}
	b := a
	b.Params = []string{"number"}

	idA, _ := mustMint(t, a)
	idB, _ := mustMint(t, b)
	if idA == idB {
		t.Fatalf("TypeScript overloads must mint different UUIDs")
	}
}

func TestCanonicalize_GoOverloadIgnored(t *testing.T) {
	// Go does not allow overloads — even if Params is set, sigHash must
	// not be appended.
	a := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         "Frob",
		Params:       []string{"int"},
	}
	canon, err := Canonicalize(a)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if want := "github.com/foo/bar/internal/widget.Frob"; canon.QualifiedName() != want {
		t.Fatalf("Go canonical form must omit sigHash: got %q want %q",
			canon.QualifiedName(), want)
	}
}

func TestCanonicalize_AnonymousFunctionsRejected(t *testing.T) {
	parts := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         "",
	}
	_, err := Canonicalize(parts)
	if !errors.Is(err, ErrAnonymousFunction) {
		t.Fatalf("expected ErrAnonymousFunction, got %v", err)
	}
}

func TestNewID_RejectsHandBuiltParts(t *testing.T) {
	// Caller skipped Canonicalize. NewID must refuse.
	parts := IdentityParts{
		Distribution: "github.com/foo/bar",
		ModulePath:   "internal/widget",
		Name:         "Frob",
	}
	_, err := NewID(EnvironmentLocal, KindFunction, parts)
	if !errors.Is(err, ErrAnonymousFunction) {
		t.Fatalf("expected ErrAnonymousFunction (zero qualifiedName), got %v", err)
	}
}

func TestNewID_RejectsUnsupportedKind(t *testing.T) {
	parts, err := Canonicalize(IdentityParts{
		Distribution: "github.com/foo/bar",
		Name:         "Frob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewID(EnvironmentLocal, KindStorage, parts); !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("expected ErrUnsupportedKind, got %v", err)
	}
}

func TestCanonicalize_UnicodeNFCEquivalence(t *testing.T) {
	// "café" — precomposed (é = U+00E9) and decomposed (e + U+0301).
	nfc := IdentityParts{Distribution: "github.com/foo/bar", Name: "caf\u00e9"}
	nfd := IdentityParts{Distribution: "github.com/foo/bar", Name: "cafe\u0301"}
	idNFC, _ := mustMint(t, nfc)
	idNFD, _ := mustMint(t, nfd)
	if idNFC != idNFD {
		t.Fatalf("NFC/NFD spellings must collapse to one UUID")
	}
}

func TestCanonicalize_PointerStripIdempotent(t *testing.T) {
	// A defensive parser may pass Container with a leading '*'. Canonicalize
	// must strip it before applying PointerReceiver wrapping.
	parts := IdentityParts{
		Distribution:    "github.com/foo/bar",
		ModulePath:      "internal/widget",
		Container:       "*OrderRequest",
		Name:            "Validate",
		PointerReceiver: true,
	}
	canon, err := Canonicalize(parts)
	if err != nil {
		t.Fatal(err)
	}
	if want := "github.com/foo/bar/internal/widget.(*OrderRequest).Validate"; canon.QualifiedName() != want {
		t.Fatalf("got %q want %q", canon.QualifiedName(), want)
	}
}

func TestNewID_NamespaceStable(t *testing.T) {
	// Pin the namespace UUID — changing it would invalidate every store
	// on disk. If this test fails, somebody changed NamespaceCvalent
	// without realising.
	want := uuid.MustParse("0a45c2e0-2c5b-4d4f-8a9b-cba1e070001a")
	if NamespaceCvalent != want {
		t.Fatalf("NamespaceCvalent changed: %v != %v", NamespaceCvalent, want)
	}
}
