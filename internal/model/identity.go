package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// NamespaceCvalent is the fixed UUID namespace used as the salt for every
// node identity in cvalent. Do not change this constant — every existing
// store on disk depends on it being stable forever.
var NamespaceCvalent = uuid.MustParse("0a45c2e0-2c5b-4d4f-8a9b-cba1e070001a")

// ErrAnonymousFunction is returned by Canonicalize and NewID when called
// on an anonymous or closure function. Anonymous functions are not nodes
// at Rung 0 — parsers must skip them rather than pass them through.
var ErrAnonymousFunction = errors.New("model: anonymous functions are not nodes at Rung 0")

// ErrUnsupportedKind is returned by NewID when called with a Kind other
// than function. The other kinds are reserved for later rungs.
var ErrUnsupportedKind = errors.New("model: only function kind is supported at Rung 0")

// IdentityParts is the only input shape accepted by Canonicalize.
//
// The qualified name is stored on the unexported `qualifiedName` field
// which only Canonicalize sets and only NewID reads. Callers cannot
// hand-format the canonical string and feed it back in — there is exactly
// one site that knows the canonical form, and it is Canonicalize.
type IdentityParts struct {
	Distribution    string
	ModulePath      string
	Container       string
	Name            string
	Params          []string
	PointerReceiver bool
	// OverloadLanguage is "java" or "typescript" when this language permits
	// overloads on parameter signature. Empty for go and python.
	OverloadLanguage string

	qualifiedName string
}

// QualifiedName returns the canonical qualified-name string previously
// computed by Canonicalize. Returns empty string if Canonicalize was not
// called or returned an error.
func (p IdentityParts) QualifiedName() string { return p.qualifiedName }

// genericTypeParams matches a trailing or embedded type-parameter list:
// `Box[T]`, `Box[int, string]`, `Func[T any]`. The expression is greedy
// from the first '[' through the matching ']'.
var genericTypeParams = regexp.MustCompile(`\[[^\]]*\]`)

// Canonicalize builds the canonical qualified-name string from
// IdentityParts. Apply rules from docs/identity-spec.md in order, then
// return the IdentityParts with the qualifiedName field populated.
//
// Returns ErrAnonymousFunction if Name is empty (anonymous functions are
// rejected at Rung 0).
func Canonicalize(p IdentityParts) (IdentityParts, error) {
	name := nfc(strings.TrimSpace(p.Name))
	if name == "" {
		return p, ErrAnonymousFunction
	}

	dist := nfc(strings.TrimSpace(p.Distribution))
	mod := nfc(strings.TrimSpace(p.ModulePath))
	container := nfc(strings.TrimSpace(p.Container))

	// Strip type-parameter lists from container and name.
	container = genericTypeParams.ReplaceAllString(container, "")
	name = genericTypeParams.ReplaceAllString(name, "")

	// Pointer-receiver wrapping. PointerReceiver is the source of truth —
	// do not infer from any leading '*' in the Container string. Strip a
	// leading '*' if a parser supplied one (defensive: pointer prefix is
	// reserved for the canonicalizer alone).
	if container != "" {
		container = strings.TrimPrefix(container, "*")
		if p.PointerReceiver {
			container = "(*" + container + ")"
		}
	}

	var b strings.Builder
	b.WriteString(dist)
	b.WriteString("/")
	b.WriteString(mod)
	b.WriteString(".")
	if container != "" {
		b.WriteString(container)
		b.WriteString(".")
	}
	b.WriteString(name)

	if needsSigHash(p.OverloadLanguage) && len(p.Params) > 0 {
		params := make([]string, len(p.Params))
		for i, t := range p.Params {
			params[i] = nfc(strings.TrimSpace(t))
		}
		joined := strings.Join(params, ",")
		sum := sha256.Sum256([]byte(joined))
		b.WriteString("#")
		b.WriteString(hex.EncodeToString(sum[:])[:8])
	}

	p.qualifiedName = b.String()
	return p, nil
}

// NewID computes the UUIDv5 for a node identity. Callers must pass an
// IdentityParts that has already been processed by Canonicalize — NewID
// reads the unexported qualifiedName field and refuses to mint an ID
// from a zero-value or hand-built IdentityParts.
//
// Returns the zero UUID and an error if env, kind, or the canonicalized
// qualified name is invalid.
func NewID(env Environment, kind Kind, parts IdentityParts) (uuid.UUID, error) {
	if env == "" {
		return uuid.Nil, errors.New("model: environment is required")
	}
	if kind != KindFunction {
		return uuid.Nil, ErrUnsupportedKind
	}
	if parts.qualifiedName == "" {
		return uuid.Nil, ErrAnonymousFunction
	}
	payload := string(env) + "|" + string(kind) + "|" + parts.qualifiedName
	return uuid.NewSHA1(NamespaceCvalent, []byte(payload)), nil
}

// MintFunctionID is a convenience for the common Rung 0 case: take an
// IdentityParts, canonicalize it, and mint the UUID, all in one call.
func MintFunctionID(env Environment, parts IdentityParts) (uuid.UUID, IdentityParts, error) {
	canonical, err := Canonicalize(parts)
	if err != nil {
		return uuid.Nil, parts, err
	}
	id, err := NewID(env, KindFunction, canonical)
	return id, canonical, err
}

func nfc(s string) string { return norm.NFC.String(s) }

func needsSigHash(lang string) bool {
	switch strings.ToLower(lang) {
	case "java", "typescript", "ts":
		return true
	}
	return false
}
