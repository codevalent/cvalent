// Package parser defines the per-file parser interface and the per-run
// state shared by every language extractor at Rung 0.
//
// All node and meta types live in internal/model. This package re-exports
// Param/ReturnSpec/ReturnValue/Field as type aliases purely so that
// extractor code can keep its short identifiers without depending on the
// model package directly. The on-the-wire shape is whatever model defines.
package parser

import (
	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

// Type aliases — every emitted struct is a model type. Do not redeclare.
type (
	FunctionNode = model.FunctionNode
	Param        = model.Param
	ReturnSpec   = model.ReturnSpec
	ReturnValue  = model.ReturnValue
	Field        = model.Field
)

// Run is the per-parser-run shared state. The Build/CLI layer constructs
// one of these for the entire walk and hands it to every per-file Parse
// call. The resolver is shared so that walking up to the same go.mod /
// package.json from sibling files is O(1) after the first hit.
type Run struct {
	Resolver *distresolver.Resolver
	Repo     *distresolver.RepoContext
}

// LanguageParser is the interface each language extractor implements.
type LanguageParser interface {
	// Parse extracts function nodes from a single source file.
	Parse(run *Run, filepath string, source []byte) ([]FunctionNode, error)
	// Language returns the language identifier (e.g. "go", "python").
	Language() string
}
