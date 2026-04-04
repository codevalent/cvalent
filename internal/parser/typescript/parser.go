package typescript

import (
	"context"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/codevalent/cvalent/internal/parser"
)

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "typescript" }

func (p *Parser) Parse(filepath string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, typescript.GetLanguage())
	if err != nil {
		return nil, err
	}

	interfaces := extractInterfaces(tree, source)
	typeAliases := extractTypeAliases(tree, source)
	// Merge into one map
	for k, v := range typeAliases {
		interfaces[k] = v
	}

	isTestFile := strings.HasSuffix(filepath, ".test.ts") ||
		strings.HasSuffix(filepath, ".spec.ts") ||
		strings.HasSuffix(filepath, ".test.tsx") ||
		strings.HasSuffix(filepath, ".spec.tsx")

	var funcs []parser.FunctionNode

	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn := extractFunction(child, source, filepath, interfaces, isTestFile, false)
			funcs = append(funcs, fn)
		case "lexical_declaration":
			// Non-exported arrow functions (const x = (...) => ...)
			funcs = append(funcs, extractLexicalDeclaration(child, source, filepath, interfaces, isTestFile, false)...)
		case "export_statement":
			funcs = append(funcs, extractExportedItems(child, source, filepath, interfaces, isTestFile)...)
		}
	}

	return funcs, nil
}

type typeInfo struct {
	fields []parser.Field
}

func extractInterfaces(tree *sitter.Node, source []byte) map[string]typeInfo {
	types := map[string]typeInfo{}
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "interface_declaration" {
			name := ""
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "type_identifier" {
					name = c.Content(source)
				}
				if c.Type() == "interface_body" {
					fields := extractInterfaceFields(c, source)
					types[name] = typeInfo{fields: fields}
				}
			}
		}
	}
	return types
}

func extractTypeAliases(tree *sitter.Node, source []byte) map[string]typeInfo {
	types := map[string]typeInfo{}
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "type_alias_declaration" {
			name := ""
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "type_identifier" {
					name = c.Content(source)
				}
				if c.Type() == "object_type" {
					fields := extractObjectTypeFields(c, source)
					types[name] = typeInfo{fields: fields}
				}
			}
		}
	}
	return types
}

func extractInterfaceFields(body *sitter.Node, source []byte) []parser.Field {
	var fields []parser.Field
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() == "property_signature" {
			name := ""
			typeStr := ""
			nullable := false
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "property_identifier" {
					name = c.Content(source)
				}
				if c.Type() == "type_annotation" {
					typeStr = extractTypeAnnotation(c, source)
				}
			}
			// Check for optional property (name?)
			content := child.Content(source)
			if strings.Contains(content, "?:") {
				nullable = true
			}
			fields = append(fields, parser.Field{Name: name, Type: typeStr, Nullable: nullable})
		}
	}
	return fields
}

func extractObjectTypeFields(body *sitter.Node, source []byte) []parser.Field {
	var fields []parser.Field
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() == "property_signature" {
			name := ""
			typeStr := ""
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "property_identifier" {
					name = c.Content(source)
				}
				if c.Type() == "type_annotation" {
					typeStr = extractTypeAnnotation(c, source)
				}
			}
			fields = append(fields, parser.Field{Name: name, Type: typeStr})
		}
	}
	return fields
}

func extractExportedItems(node *sitter.Node, source []byte, filepath string, types map[string]typeInfo, isTestFile bool) []parser.FunctionNode {
	var funcs []parser.FunctionNode
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn := extractFunction(child, source, filepath, types, isTestFile, true)
			funcs = append(funcs, fn)
		case "lexical_declaration":
			funcs = append(funcs, extractLexicalDeclaration(child, source, filepath, types, isTestFile, true)...)
		case "class_declaration":
			funcs = append(funcs, extractClassMembers(child, source, filepath, types, isTestFile)...)
		}
	}
	return funcs
}

func extractFunction(node *sitter.Node, source []byte, filepath string, types map[string]typeInfo, isTestFile bool, exported bool) parser.FunctionNode {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" {
			name = c.Content(source)
			break
		}
	}

	params := extractFormalParams(node, source, types)
	returns := extractReturnType(node, source, types)

	tag := "application"
	if isTestFile {
		tag = "test"
	}

	completeness := "full"
	if hasAnyType(params, returns) {
		completeness = "partial"
	}

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        name,
		File:                 filepath,
		Language:             "typescript",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 "function",
		Exported:             exported,
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: completeness,
	}
}

func extractLexicalDeclaration(node *sitter.Node, source []byte, filepath string, types map[string]typeInfo, isTestFile bool, exported bool) []parser.FunctionNode {
	var funcs []parser.FunctionNode
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "variable_declarator" {
			name := ""
			var arrowFn *sitter.Node
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "identifier" {
					name = c.Content(source)
				}
				if c.Type() == "arrow_function" {
					arrowFn = c
				}
			}
			if arrowFn != nil {
				fn := extractArrowFunction(arrowFn, name, source, filepath, types, isTestFile, exported)
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs
}

func extractArrowFunction(node *sitter.Node, name string, source []byte, filepath string, types map[string]typeInfo, isTestFile bool, exported bool) parser.FunctionNode {
	params := extractFormalParams(node, source, types)
	returns := extractReturnType(node, source, types)

	tag := "application"
	if isTestFile {
		tag = "test"
	}

	completeness := "full"
	if hasAnyType(params, returns) {
		completeness = "partial"
	}

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        name,
		File:                 filepath,
		Language:             "typescript",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 "function",
		Exported:             exported,
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: completeness,
	}
}

func extractClassMembers(classNode *sitter.Node, source []byte, filepath string, types map[string]typeInfo, isTestFile bool) []parser.FunctionNode {
	className := ""
	for i := 0; i < int(classNode.NamedChildCount()); i++ {
		c := classNode.NamedChild(i)
		if c.Type() == "type_identifier" {
			className = c.Content(source)
			break
		}
	}

	var body *sitter.Node
	for i := 0; i < int(classNode.NamedChildCount()); i++ {
		c := classNode.NamedChild(i)
		if c.Type() == "class_body" {
			body = c
			break
		}
	}
	if body == nil {
		return nil
	}

	var funcs []parser.FunctionNode
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() == "method_definition" {
			fn := extractMethodDef(child, className, source, filepath, types, isTestFile)
			funcs = append(funcs, fn)
		}
	}
	return funcs
}

func extractMethodDef(node *sitter.Node, className string, source []byte, filepath string, types map[string]typeInfo, isTestFile bool) parser.FunctionNode {
	name := ""
	exported := false
	isConstructor := false

	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		switch c.Type() {
		case "property_identifier":
			name = c.Content(source)
			if name == "constructor" {
				isConstructor = true
			}
		case "accessibility_modifier":
			if c.Content(source) == "public" {
				exported = true
			}
		}
	}

	params := extractFormalParams(node, source, types)
	returns := extractReturnType(node, source, types)

	kind := "method"
	if isConstructor {
		kind = "constructor"
		returns = parser.ReturnSpec{Values: []parser.ReturnValue{}}
	}

	qualifiedName := className + "." + name

	tag := "application"
	if isTestFile {
		tag = "test"
	}

	completeness := "full"
	if hasAnyType(params, returns) {
		completeness = "partial"
	}

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        qualifiedName,
		File:                 filepath,
		Language:             "typescript",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 kind,
		Receiver:             className,
		Exported:             exported,
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: completeness,
	}
}

func extractFormalParams(node *sitter.Node, source []byte, types map[string]typeInfo) []parser.Param {
	params := []parser.Param{}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "formal_parameters" {
			for j := 0; j < int(c.NamedChildCount()); j++ {
				p := c.NamedChild(j)
				switch p.Type() {
				case "required_parameter", "optional_parameter":
					param := extractParam(p, source, types)
					params = append(params, param)
				}
			}
			break
		}
	}
	return params
}

func extractParam(node *sitter.Node, source []byte, types map[string]typeInfo) parser.Param {
	name := ""
	typeStr := ""
	nullable := false
	hasDefault := false
	defaultValue := ""

	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		switch c.Type() {
		case "identifier":
			name = c.Content(source)
		case "type_annotation":
			typeStr = extractTypeAnnotation(c, source)
		}
	}

	// Check for default value (= value)
	content := node.Content(source)
	if idx := strings.Index(content, "="); idx > 0 {
		hasDefault = true
		defaultValue = strings.TrimSpace(content[idx+1:])
	}

	// Check for optional (?)
	if node.Type() == "optional_parameter" {
		nullable = true
	}

	// Check for union with null/undefined
	if strings.Contains(typeStr, "| null") || strings.Contains(typeStr, "| undefined") {
		nullable = true
	}

	p := parser.Param{
		Name:     name,
		Type:     typeStr,
		Nullable: nullable,
	}

	if hasDefault {
		_ = defaultValue // stored for future use
	}

	// One-hop expansion
	baseType := typeStr
	if ti, ok := types[baseType]; ok {
		p.Expanded = ti.fields
	}

	return p
}

func extractReturnType(node *sitter.Node, source []byte, types map[string]typeInfo) parser.ReturnSpec {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "type_annotation" {
			typeStr := extractTypeAnnotation(c, source)
			if typeStr == "void" {
				return parser.ReturnSpec{Values: []parser.ReturnValue{}}
			}

			rv := parser.ReturnValue{Type: typeStr}

			// Nullable if union with null/undefined
			if strings.Contains(typeStr, "| null") || strings.Contains(typeStr, "| undefined") {
				rv.Nullable = true
			}

			// One-hop expansion
			if ti, ok := types[typeStr]; ok {
				rv.Expanded = ti.fields
			}

			return parser.ReturnSpec{Values: []parser.ReturnValue{rv}}
		}
	}
	return parser.ReturnSpec{Values: []parser.ReturnValue{}}
}

func extractTypeAnnotation(node *sitter.Node, source []byte) string {
	// type_annotation contains ": type" — extract everything after the colon
	content := node.Content(source)
	if strings.HasPrefix(content, ":") {
		return strings.TrimSpace(content[1:])
	}
	return content
}

func hasAnyType(params []parser.Param, returns parser.ReturnSpec) bool {
	for _, p := range params {
		if p.Type == "any" {
			return true
		}
	}
	for _, r := range returns.Values {
		if r.Type == "any" {
			return true
		}
	}
	return false
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}
