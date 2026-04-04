package golang

import (
	"context"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/codevalent/cvalent/internal/parser"
)

// Parser extracts function nodes from Go source files using tree-sitter.
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "go" }

func (p *Parser) Parse(filepath string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, golang.GetLanguage())
	if err != nil {
		return nil, err
	}

	pkg := extractPackage(tree, source)
	structs := extractStructs(tree, source)
	isTestFile := strings.HasSuffix(filepath, "_test.go")

	var funcs []parser.FunctionNode

	// Walk top-level declarations
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn := extractFunction(child, source, filepath, pkg, structs, isTestFile)
			funcs = append(funcs, fn)
		case "method_declaration":
			fn := extractMethod(child, source, filepath, pkg, structs, isTestFile)
			funcs = append(funcs, fn)
		}
	}

	return funcs, nil
}

func extractPackage(tree *sitter.Node, source []byte) string {
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "package_clause" {
			// package_clause -> package_identifier (may be field "name" or a named child)
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(source)
			}
			// Fallback: look for package_identifier child directly
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "package_identifier" {
					return c.Content(source)
				}
			}
		}
	}
	return ""
}

// structInfo holds the fields of a struct found in the same file.
type structInfo struct {
	name   string
	fields []parser.Field
}

func extractStructs(tree *sitter.Node, source []byte) map[string]structInfo {
	structs := map[string]structInfo{}
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() != "type_declaration" {
			continue
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			spec := child.NamedChild(j)
			if spec.Type() != "type_spec" {
				continue
			}
			nameNode := spec.ChildByFieldName("name")
			typeNode := spec.ChildByFieldName("type")
			if nameNode == nil || typeNode == nil || typeNode.Type() != "struct_type" {
				continue
			}
			name := nameNode.Content(source)
			fields := extractStructFields(typeNode, source)
			structs[name] = structInfo{name: name, fields: fields}
		}
	}
	return structs
}

func extractStructFields(structNode *sitter.Node, source []byte) []parser.Field {
	var fields []parser.Field
	fieldList := structNode.ChildByFieldName("body") // field_declaration_list
	if fieldList == nil {
		// Try finding it as named child
		for i := 0; i < int(structNode.NamedChildCount()); i++ {
			c := structNode.NamedChild(i)
			if c.Type() == "field_declaration_list" {
				fieldList = c
				break
			}
		}
	}
	if fieldList == nil {
		return fields
	}
	for i := 0; i < int(fieldList.NamedChildCount()); i++ {
		fieldDecl := fieldList.NamedChild(i)
		if fieldDecl.Type() != "field_declaration" {
			continue
		}
		typeNode := fieldDecl.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		typeStr := typeNode.Content(source)
		nullable := isNullableType(typeNode, source)

		// Collect all field names (Go can have `X, Y int`)
		nameNode := fieldDecl.ChildByFieldName("name")
		if nameNode != nil {
			f := parser.Field{Name: nameNode.Content(source), Type: typeStr}
			if nullable {
				f.Nullable = true
			}
			fields = append(fields, f)
		}
	}
	return fields
}

func extractFunction(node *sitter.Node, source []byte, filepath, pkg string, structs map[string]structInfo, isTestFile bool) parser.FunctionNode {
	nameNode := node.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = nameNode.Content(source)
	}

	params := extractParams(node.ChildByFieldName("parameters"), source, structs)
	returns := extractReturns(node.ChildByFieldName("result"), source, structs)

	tag := "application"
	if isTestFile && isTestFuncName(name) {
		tag = "test"
	}

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        pkg + "." + name,
		File:                 filepath,
		Package:              pkg,
		Language:             "go",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 "function",
		Exported:             isExported(name),
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: "full",
	}
}

func extractMethod(node *sitter.Node, source []byte, filepath, pkg string, structs map[string]structInfo, isTestFile bool) parser.FunctionNode {
	nameNode := node.ChildByFieldName("name")
	name := ""
	if nameNode != nil {
		name = nameNode.Content(source)
	}

	receiver := extractReceiver(node, source)
	receiverTypeName := strings.TrimPrefix(receiver, "*")

	params := extractParams(node.ChildByFieldName("parameters"), source, structs)
	returns := extractReturns(node.ChildByFieldName("result"), source, structs)

	qualifiedName := pkg + "." + receiverTypeName + "." + name

	tag := "application"
	if isTestFile && isTestFuncName(name) {
		tag = "test"
	}

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        qualifiedName,
		File:                 filepath,
		Package:              pkg,
		Language:             "go",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 "method",
		Receiver:             receiver,
		Exported:             isExported(name),
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: "full",
	}
}

func extractReceiver(node *sitter.Node, source []byte) string {
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode == nil {
		return ""
	}
	// parameter_list with one parameter_declaration inside
	for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
		param := receiverNode.NamedChild(i)
		if param.Type() == "parameter_declaration" {
			typeNode := param.ChildByFieldName("type")
			if typeNode != nil {
				return typeNode.Content(source)
			}
		}
	}
	return ""
}

func extractParams(paramsNode *sitter.Node, source []byte, structs map[string]structInfo) []parser.Param {
	if paramsNode == nil {
		return []parser.Param{}
	}

	var params []parser.Param
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(i)
		switch child.Type() {
		case "parameter_declaration":
			p := extractSingleParam(child, source, structs, false)
			params = append(params, p...)
		case "variadic_parameter_declaration":
			p := extractVariadicParam(child, source)
			params = append(params, p)
		}
	}
	return params
}

func extractSingleParam(node *sitter.Node, source []byte, structs map[string]structInfo, isVariadic bool) []parser.Param {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}

	typeStr := typeNode.Content(source)
	nullable := isNullableType(typeNode, source)

	// Collect all names (Go allows `a, b int`)
	var names []string
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		names = append(names, nameNode.Content(source))
	}

	if len(names) == 0 {
		names = []string{""}
	}

	var params []parser.Param
	for _, name := range names {
		p := parser.Param{Name: name}

		// Try struct expansion
		baseType := strings.TrimPrefix(typeStr, "*")
		if si, ok := structs[baseType]; ok {
			p.Type = typeStr
			p.Expanded = si.fields
		} else {
			p.Type = typeStr
		}

		if nullable {
			p.Nullable = true
		}
		if isVariadic {
			p.Variadic = true
		}

		// If the base type is not in same-file structs and is a non-primitive complex type,
		// use type_expr instead of type (for types we can't expand)
		if shouldUseTypeExpr(typeStr, structs) {
			p.Type = ""
			p.TypeExpr = typeStr
		}

		params = append(params, p)
	}
	return params
}

func extractVariadicParam(node *sitter.Node, source []byte) parser.Param {
	nameNode := node.ChildByFieldName("name")
	typeNode := node.ChildByFieldName("type")

	name := ""
	if nameNode != nil {
		name = nameNode.Content(source)
	}

	typeStr := ""
	if typeNode != nil {
		typeStr = "..." + typeNode.Content(source)
	}

	return parser.Param{
		Name:     name,
		Type:     typeStr,
		Variadic: true,
	}
}

func extractReturns(resultNode *sitter.Node, source []byte, structs map[string]structInfo) parser.ReturnSpec {
	if resultNode == nil {
		return parser.ReturnSpec{Values: []parser.ReturnValue{}}
	}

	switch resultNode.Type() {
	case "parameter_list":
		// Multiple returns: (Type1, Type2) or (name Type1, name Type2)
		var values []parser.ReturnValue
		for i := 0; i < int(resultNode.NamedChildCount()); i++ {
			child := resultNode.NamedChild(i)
			if child.Type() == "parameter_declaration" {
				rv := extractReturnValue(child, source, structs)
				values = append(values, rv)
			}
		}
		return parser.ReturnSpec{
			Multiple: len(values) > 1,
			Values:   values,
		}
	default:
		// Single return type (no parens)
		rv := singleReturnValue(resultNode, source, structs)
		return parser.ReturnSpec{Values: []parser.ReturnValue{rv}}
	}
}

func extractReturnValue(node *sitter.Node, source []byte, structs map[string]structInfo) parser.ReturnValue {
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("name")

	rv := parser.ReturnValue{}
	if nameNode != nil {
		rv.Name = nameNode.Content(source)
	}
	if typeNode != nil {
		typeStr := typeNode.Content(source)
		rv.Nullable = isNullableType(typeNode, source) || typeStr == "error"

		baseType := strings.TrimPrefix(typeStr, "*")
		if si, ok := structs[baseType]; ok {
			rv.Type = typeStr
			rv.Expanded = si.fields
		} else if shouldUseTypeExpr(typeStr, structs) {
			rv.TypeExpr = typeStr
		} else {
			rv.Type = typeStr
		}
	}
	return rv
}

func singleReturnValue(node *sitter.Node, source []byte, structs map[string]structInfo) parser.ReturnValue {
	typeStr := node.Content(source)
	rv := parser.ReturnValue{}
	rv.Nullable = isNullableType(node, source) || typeStr == "error"

	baseType := strings.TrimPrefix(typeStr, "*")
	if si, ok := structs[baseType]; ok {
		rv.Type = typeStr
		rv.Expanded = si.fields
	} else if shouldUseTypeExpr(typeStr, structs) {
		rv.TypeExpr = typeStr
	} else {
		rv.Type = typeStr
	}
	return rv
}

func isNullableType(node *sitter.Node, source []byte) bool {
	if node.Type() == "pointer_type" {
		return true
	}
	content := node.Content(source)
	return strings.HasPrefix(content, "*") || content == "error"
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

func isTestFuncName(name string) bool {
	return strings.HasPrefix(name, "Test") ||
		strings.HasPrefix(name, "Benchmark") ||
		strings.HasPrefix(name, "Example")
}

// shouldUseTypeExpr returns true if a type should be recorded as type_expr
// rather than type — when it references a non-primitive type not defined in the same file.
func shouldUseTypeExpr(typeStr string, structs map[string]structInfo) bool {
	// Strip pointer/slice prefixes to get base type
	base := typeStr
	for strings.HasPrefix(base, "*") || strings.HasPrefix(base, "[]") {
		base = strings.TrimPrefix(base, "*")
		base = strings.TrimPrefix(base, "[]")
	}

	// Primitives and builtins are always type (not type_expr)
	if isPrimitive(base) {
		return false
	}

	// If it's a qualified type (has dot), it's external — type_expr
	if strings.Contains(base, ".") {
		// But standard library types like testing.T are still "type"
		return false
	}

	// Interface types inline
	if base == "interface{}" {
		return false
	}

	// If defined in same file structs, use type (will be expanded)
	if _, ok := structs[base]; ok {
		return false
	}

	// Unknown type not in same file — type_expr
	return true
}

func isPrimitive(t string) bool {
	switch t {
	case "bool", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune", "float32", "float64", "complex64", "complex128",
		"error", "any", "interface{}", "":
		return true
	}
	return false
}
