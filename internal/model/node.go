// Package model defines the unified Node type and the canonical identity
// surface used by every parser, the migrator, and the parity harness.
//
// At Rung 0 the only node Kind that exists is "function" (with method
// being a sub-shape carried in FunctionMeta.Receiver). Every UUID is
// produced by identity.NewID after the qualified name has been built by
// identity.Canonicalize. There is no other supported way to mint an
// identity in cvalent.
package model

import (
	"github.com/google/uuid"
)

// IdentitySource records how a Node's distribution was resolved.
//
// This is surfaced on parser output and on every node row in the store so
// that the friction surface (internal/friction) can decide whether a
// cross-repo edge target is "in scope" or "out of scope" for the OSS
// install — and so that hosted (Rung 1+) can re-mint repo-fallback
// identities into real distributions without churning UUIDs that already
// have a real distribution.
type IdentitySource string

const (
	// IdentityFromDistribution means the node's distribution was resolved
	// from a real manifest file (go.mod, package.json, pyproject.toml,
	// pom.xml, build.gradle).
	IdentityFromDistribution IdentitySource = "distribution"

	// IdentityFromRepoFallback means no manifest was found but a git
	// remote was, so the distribution is "repo:<account>/<repo>".
	IdentityFromRepoFallback IdentitySource = "repo_fallback"

	// IdentityFromRepoFallbackNoRemote means no manifest AND no git
	// remote — distribution is "repo:<basename>".
	IdentityFromRepoFallbackNoRemote IdentitySource = "repo_fallback_no_remote"
)

// Kind is the discriminator for what slot of meta a Node carries.
//
// At Rung 0 only KindFunction is populated. The other constants are
// declared so that schema migrations and friction code can refer to them
// by name from day one — Rung 4 will populate them.
type Kind string

const (
	KindFunction     Kind = "function"
	KindPipelineStep Kind = "pipeline_step"
	KindStorage      Kind = "storage"
	KindEndpoint     Kind = "endpoint"
)

// Environment is the scoping field that becomes meaningful at Rung 1.
//
// At Rung 0 every node is stamped EnvironmentLocal. The hosted store will
// introduce real environment names without re-minting any UUIDs that
// already exist in the local environment, because environment is part of
// the canonical input to NewID.
type Environment string

const EnvironmentLocal Environment = "local"

// Node is the unified node type. Every row in the `nodes` table maps 1:1
// to a Node value. Kind-specific fields live in the corresponding meta
// struct (FunctionMeta, etc.) and are joined at the domain layer.
type Node struct {
	ID             uuid.UUID      `json:"id"`
	Environment    Environment    `json:"environment"`
	Kind           Kind           `json:"kind"`
	QualifiedName  string         `json:"qualified_name"`
	Name           string         `json:"name"`
	Distribution   string         `json:"distribution"`
	ModulePath     string         `json:"package"`
	Language       string         `json:"language"`
	File           string         `json:"file"`
	IdentitySource IdentitySource `json:"identity_source"`
}

// FunctionMeta is the kind=function slot. It is stored 1:1 against
// node_function_meta in the store and joined into FunctionNode for the
// domain layer.
type FunctionMeta struct {
	StartLine            int        `json:"start_line"`
	EndLine              int        `json:"end_line"`
	Exported             bool       `json:"exported"`
	Tag                  string     `json:"tag"` // "application" or "test"
	Receiver             string     `json:"receiver,omitempty"`
	PointerReceiver      bool       `json:"pointer_receiver,omitempty"`
	Params               []Param    `json:"params"`
	Returns              ReturnSpec `json:"returns"`
	ContractCompleteness string     `json:"contract_completeness"`
}

// FunctionNode is the domain join of Node + FunctionMeta. Parsers emit
// []FunctionNode at Rung 0 (the only kind that exists).
type FunctionNode struct {
	Node
	FunctionMeta
}

// Param represents a function parameter with optional one-hop expansion.
type Param struct {
	Name     string  `json:"name"`
	Type     string  `json:"type,omitempty"`
	TypeExpr string  `json:"type_expr,omitempty"`
	Nullable bool    `json:"nullable,omitempty"`
	Variadic bool    `json:"variadic,omitempty"`
	Expanded []Field `json:"expanded,omitempty"`
}

// ReturnSpec describes a function's return values.
type ReturnSpec struct {
	Multiple bool          `json:"multiple,omitempty"`
	Values   []ReturnValue `json:"values"`
}

// ReturnValue represents a single return type.
type ReturnValue struct {
	Name     string  `json:"name,omitempty"`
	Type     string  `json:"type,omitempty"`
	TypeExpr string  `json:"type_expr,omitempty"`
	Nullable bool    `json:"nullable,omitempty"`
	Expanded []Field `json:"expanded,omitempty"`
}

// Field is a struct/class field used for one-hop expansion of Param.Type
// and ReturnValue.Type.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable,omitempty"`
}
