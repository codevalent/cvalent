package parser

// FunctionNode represents an extracted function with its contract.
type FunctionNode struct {
	Name                 string     `json:"name"`
	QualifiedName        string     `json:"qualified_name"`
	File                 string     `json:"file"`
	Package              string     `json:"package"`
	Language             string     `json:"language"`
	StartLine            int        `json:"start_line"`
	EndLine              int        `json:"end_line"`
	Kind                 string     `json:"kind"`                  // "function", "method"
	Receiver             string     `json:"receiver,omitempty"`    // for methods: "OrderRequest", "*OrderRequest"
	Exported             bool       `json:"exported"`
	Tag                  string     `json:"tag"`                   // "application" or "test"
	Parameters           []Param    `json:"parameters"`
	Returns              ReturnSpec `json:"returns"`
	ContractCompleteness string     `json:"contract_completeness"` // "full", "partial", "inferred"
}

// Param represents a function parameter with optional type expansion.
type Param struct {
	Name     string  `json:"name"`
	Type     string  `json:"type,omitempty"`
	TypeExpr string  `json:"type_expr,omitempty"` // raw type expression when not expandable
	Nullable bool    `json:"nullable,omitempty"`
	Variadic bool    `json:"variadic,omitempty"`
	Expanded []Field `json:"expanded,omitempty"` // one-hop struct expansion
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

// Field represents a struct/class field for one-hop expansion.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable,omitempty"`
}

// LanguageParser is the interface each language extractor implements.
type LanguageParser interface {
	// Parse extracts function nodes from a single source file.
	Parse(filepath string, source []byte) ([]FunctionNode, error)
	// Language returns the language identifier (e.g., "go", "python").
	Language() string
}
