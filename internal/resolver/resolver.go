package resolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/codevalent/cvalent/internal/parser"
)

// CallEdge represents a resolved function call.
type CallEdge struct {
	CallerFile      string
	CallerName      string
	CallerQualified string
	CalleeFile      string
	CalleeName      string
	CalleeQualified string
}

// Resolve takes parsed function nodes and source files, finds call sites,
// and returns resolved CALLS edges.
func Resolve(root string, nodes []parser.FunctionNode) ([]CallEdge, error) {
	// Build symbol index: name -> []FunctionNode (multiple files can define same name)
	nameIndex := map[string][]parser.FunctionNode{}
	for _, n := range nodes {
		nameIndex[n.Name] = append(nameIndex[n.Name], n)
	}

	// For each source file, find call expressions and match against index
	var edges []CallEdge
	fileNodes := groupByFile(nodes)

	for file, fileFuncs := range fileNodes {
		fullPath := filepath.Join(root, file)
		source, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		lang := languageFromFile(file)
		if lang == nil {
			continue
		}

		tree, err := sitter.ParseCtx(context.Background(), source, lang)
		if err != nil {
			continue
		}

		// For each function in this file, find call sites within its body
		for _, caller := range fileFuncs {
			// Find the function's AST node by line number
			callees := findCallsInFunction(tree, source, caller, nameIndex)
			for _, callee := range callees {
				// Don't create self-edges
				if callee.QualifiedName == caller.QualifiedName {
					continue
				}
				edges = append(edges, CallEdge{
					CallerFile:      caller.File,
					CallerName:      caller.Name,
					CallerQualified: caller.QualifiedName,
					CalleeFile:      callee.File,
					CalleeName:      callee.Name,
					CalleeQualified: callee.QualifiedName,
				})
			}
		}
	}

	// Deduplicate edges
	return dedup(edges), nil
}

func groupByFile(nodes []parser.FunctionNode) map[string][]parser.FunctionNode {
	m := map[string][]parser.FunctionNode{}
	for _, n := range nodes {
		m[n.File] = append(m[n.File], n)
	}
	return m
}

func languageFromFile(file string) *sitter.Language {
	ext := filepath.Ext(file)
	switch ext {
	case ".go":
		return golang.GetLanguage()
	case ".java":
		return java.GetLanguage()
	case ".ts", ".tsx":
		return typescript.GetLanguage()
	case ".py":
		return python.GetLanguage()
	default:
		return nil
	}
}

func findCallsInFunction(tree *sitter.Node, source []byte, caller parser.FunctionNode, index map[string][]parser.FunctionNode) []parser.FunctionNode {
	var callees []parser.FunctionNode
	seen := map[string]bool{}

	// Walk entire tree looking for call expressions within the caller's line range
	walkTree(tree, source, func(node *sitter.Node) {
		line := int(node.StartPoint().Row) + 1
		if line < caller.StartLine || line > caller.EndLine {
			return
		}

		calledName := ""
		switch node.Type() {
		case "call_expression":
			// Go, TypeScript, Java: call_expression -> function child
			calledName = extractCallTarget(node, source)
		case "call":
			// Python: call -> function child
			calledName = extractCallTarget(node, source)
		}

		if calledName == "" {
			return
		}

		// Match against symbol index
		candidates, ok := index[calledName]
		if !ok {
			return
		}

		for _, c := range candidates {
			key := c.QualifiedName
			if !seen[key] {
				seen[key] = true
				callees = append(callees, c)
			}
		}
	})

	return callees
}

func extractCallTarget(node *sitter.Node, source []byte) string {
	// The first child is usually the function name/expression
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		switch c.Type() {
		case "identifier":
			return c.Content(source)
		case "member_expression", "attribute", "field_access":
			// method call: obj.method() — extract just the method name
			return lastIdentifier(c, source)
		case "selector_expression":
			// Go: pkg.Func() — extract the function name
			return lastIdentifier(c, source)
		}
	}
	return ""
}

func lastIdentifier(node *sitter.Node, source []byte) string {
	last := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" || c.Type() == "property_identifier" || c.Type() == "field_identifier" {
			last = c.Content(source)
		}
	}
	return last
}

func walkTree(node *sitter.Node, source []byte, fn func(*sitter.Node)) {
	fn(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkTree(node.NamedChild(i), source, fn)
	}
}

func dedup(edges []CallEdge) []CallEdge {
	seen := map[string]bool{}
	var result []CallEdge
	for _, e := range edges {
		key := e.CallerQualified + "->" + e.CalleeQualified
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// SkipLanguage checks if a file extension should not be resolved cross-file.
func SkipLanguage(ext string) bool {
	switch ext {
	case ".go", ".java", ".ts", ".tsx", ".py":
		return false
	default:
		return true
	}
}

func init() {
	// Ensure unused imports don't cause errors
	_ = strings.HasPrefix
}
