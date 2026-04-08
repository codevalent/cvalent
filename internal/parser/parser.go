package parser

import (
	"fmt"

	"github.com/codevalent/cvalent/internal/model"
)

// Mint canonicalizes IdentityParts and produces a Node populated with
// every field that depends on identity resolution. Per-language parsers
// fill in the language, file, kind, and FunctionMeta fields after
// calling Mint.
//
// Mint is the only function in this package that calls model.NewID.
// Parsers must not call NewID directly — that way the only sites that
// can mint a UUID are model.NewID itself and parser.Mint, which makes
// "did the parser mint identities correctly?" answerable in one place.
func Mint(parts model.IdentityParts, language, file string, source model.IdentitySource) (model.Node, error) {
	id, canon, err := model.MintFunctionID(model.EnvironmentLocal, parts)
	if err != nil {
		return model.Node{}, fmt.Errorf("parser.Mint(%s): %w", parts.Name, err)
	}
	return model.Node{
		ID:             id,
		Environment:    model.EnvironmentLocal,
		Kind:           model.KindFunction,
		QualifiedName:  canon.QualifiedName(),
		Name:           parts.Name,
		Distribution:   parts.Distribution,
		ModulePath:     parts.ModulePath,
		Language:       language,
		File:           file,
		IdentitySource: source,
	}, nil
}
