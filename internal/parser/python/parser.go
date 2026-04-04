package python

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/codevalent/cvalent/internal/parser"
)

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "python" }

func (p *Parser) Parse(filepath string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, python.GetLanguage())
	if err != nil {
		return nil, err
	}

	classes := extractClasses(tree, source)
	isTestFile := strings.HasPrefix(lastPathComponent(filepath), "test_") ||
		strings.HasSuffix(filepath, "_test.py")

	var funcs []parser.FunctionNode

	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "function_definition":
			fn := extractFunction(child, source, filepath, "", classes, isTestFile)
			funcs = append(funcs, fn)
		case "decorated_definition":
			fn := extractDecoratedFunc(child, source, filepath, "", classes, isTestFile)
			if fn != nil {
				funcs = append(funcs, *fn)
			}
		case "class_definition":
			funcs = append(funcs, extractClassMethods(child, source, filepath, classes, isTestFile)...)
		}
	}

	return funcs, nil
}

type classInfo struct {
	name   string
	fields []parser.Field
}

func extractClasses(tree *sitter.Node, source []byte) map[string]classInfo {
	classes := map[string]classInfo{}
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "class_definition" || child.Type() == "decorated_definition" {
			node := child
			if child.Type() == "decorated_definition" {
				node = findChild(child, "class_definition")
				if node == nil {
					continue
				}
			}
			name := findChildContent(node, "identifier", source)
			body := findChild(node, "block")
			if body == nil {
				continue
			}
			fields := extractClassFields(body, source)
			classes[name] = classInfo{name: name, fields: fields}
		}
	}
	return classes
}

func extractClassFields(body *sitter.Node, source []byte) []parser.Field {
	var fields []parser.Field
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() == "expression_statement" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				expr := child.NamedChild(j)
				if expr.Type() == "assignment" {
					name := ""
					typeStr := ""
					for k := 0; k < int(expr.ChildCount()); k++ {
						c := expr.Child(k)
						if c.Type() == "identifier" && name == "" {
							name = c.Content(source)
						}
						if c.Type() == "type" {
							typeStr = c.Content(source)
						}
					}
					if name != "" && typeStr != "" {
						nullable := isNullableTypeStr(typeStr)
						fields = append(fields, parser.Field{Name: name, Type: typeStr, Nullable: nullable})
					}
				}
			}
		}
	}
	return fields
}

func extractFunction(node *sitter.Node, source []byte, filepath, className string, classes map[string]classInfo, isTestFile bool) parser.FunctionNode {
	name := findChildContent(node, "identifier", source)
	params := extractParams(node, source, classes, className != "")
	returns := extractReturnType(node, source, classes)

	exported := !strings.HasPrefix(name, "_") || strings.HasPrefix(name, "__")
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		exported = true // dunder methods
	}

	tag := "application"
	if isTestFile && strings.HasPrefix(name, "test_") {
		tag = "test"
	}

	kind := "function"
	qualifiedName := name
	receiver := ""
	if className != "" {
		kind = "method"
		qualifiedName = className + "." + name
		receiver = className
	}

	completeness := determineCompleteness(params, returns)

	return parser.FunctionNode{
		Name:                 name,
		QualifiedName:        qualifiedName,
		File:                 filepath,
		Language:             "python",
		StartLine:            int(node.StartPoint().Row) + 1,
		EndLine:              int(node.EndPoint().Row) + 1,
		Kind:                 kind,
		Receiver:             receiver,
		Exported:             exported,
		Tag:                  tag,
		Parameters:           params,
		Returns:              returns,
		ContractCompleteness: completeness,
	}
}

func extractDecoratedFunc(node *sitter.Node, source []byte, filepath, className string, classes map[string]classInfo, isTestFile bool) *parser.FunctionNode {
	funcNode := findChild(node, "function_definition")
	if funcNode == nil {
		return nil
	}
	fn := extractFunction(funcNode, source, filepath, className, classes, isTestFile)
	// Adjust line numbers to include decorator
	fn.StartLine = int(node.StartPoint().Row) + 1
	return &fn
}

func extractClassMethods(classNode *sitter.Node, source []byte, filepath string, classes map[string]classInfo, isTestFile bool) []parser.FunctionNode {
	className := findChildContent(classNode, "identifier", source)
	body := findChild(classNode, "block")
	if body == nil {
		return nil
	}

	var funcs []parser.FunctionNode
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "function_definition":
			fn := extractFunction(child, source, filepath, className, classes, isTestFile)
			funcs = append(funcs, fn)
		case "decorated_definition":
			fn := extractDecoratedFunc(child, source, filepath, className, classes, isTestFile)
			if fn != nil {
				fn.Kind = "method"
				fn.Receiver = className
				fn.QualifiedName = className + "." + fn.Name
				funcs = append(funcs, *fn)
			}
		}
	}
	return funcs
}

func extractParams(node *sitter.Node, source []byte, classes map[string]classInfo, isMethod bool) []parser.Param {
	params := []parser.Param{}
	paramsNode := findChild(node, "parameters")
	if paramsNode == nil {
		return params
	}

	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(i)
		switch child.Type() {
		case "identifier":
			name := child.Content(source)
			if isMethod && name == "self" {
				continue
			}
			// Untyped parameter — inferred
			params = append(params, parser.Param{Name: name})
		case "typed_parameter":
			p := extractTypedParam(child, source, classes)
			if isMethod && p.Name == "self" {
				continue
			}
			params = append(params, p)
		case "typed_default_parameter":
			p := extractTypedDefaultParam(child, source, classes)
			if isMethod && p.Name == "self" {
				continue
			}
			params = append(params, p)
		case "default_parameter":
			p := extractDefaultParam(child, source)
			if isMethod && p.Name == "self" {
				continue
			}
			params = append(params, p)
		}
	}
	return params
}

func extractTypedParam(node *sitter.Node, source []byte, classes map[string]classInfo) parser.Param {
	name := ""
	typeStr := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c.Type() == "identifier" && name == "" {
			name = c.Content(source)
		}
		if c.Type() == "type" {
			typeStr = c.Content(source)
		}
	}

	p := parser.Param{Name: name, Type: typeStr}
	p.Nullable = isNullableTypeStr(typeStr)

	// One-hop expansion
	baseType := typeStr
	if ci, ok := classes[baseType]; ok {
		p.Expanded = ci.fields
	}

	return p
}

func extractTypedDefaultParam(node *sitter.Node, source []byte, classes map[string]classInfo) parser.Param {
	name := ""
	typeStr := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c.Type() == "identifier" && name == "" {
			name = c.Content(source)
		}
		if c.Type() == "type" {
			typeStr = c.Content(source)
		}
	}

	p := parser.Param{Name: name, Type: typeStr}
	p.Nullable = isNullableTypeStr(typeStr)

	if ci, ok := classes[typeStr]; ok {
		p.Expanded = ci.fields
	}

	return p
}

func extractDefaultParam(node *sitter.Node, source []byte) parser.Param {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" && name == "" {
			name = c.Content(source)
		}
	}
	return parser.Param{Name: name}
}

func extractReturnType(node *sitter.Node, source []byte, classes map[string]classInfo) parser.ReturnSpec {
	// Look for -> type annotation
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c.Type() == "type" {
			typeStr := c.Content(source)
			if typeStr == "None" {
				return parser.ReturnSpec{Values: []parser.ReturnValue{}}
			}

			rv := parser.ReturnValue{Type: typeStr}
			rv.Nullable = isNullableTypeStr(typeStr)

			if ci, ok := classes[typeStr]; ok {
				rv.Expanded = ci.fields
			}

			return parser.ReturnSpec{Values: []parser.ReturnValue{rv}}
		}
	}
	// No return annotation
	return parser.ReturnSpec{Values: []parser.ReturnValue{}}
}

func determineCompleteness(params []parser.Param, returns parser.ReturnSpec) string {
	for _, p := range params {
		if p.Type == "" && p.TypeExpr == "" {
			return "inferred"
		}
	}
	return "full"
}

func isNullableTypeStr(t string) bool {
	return strings.HasPrefix(t, "Optional") ||
		strings.Contains(t, "| None") ||
		strings.Contains(t, "None |") ||
		(strings.HasPrefix(t, "Union[") && strings.Contains(t, "None"))
}

func findChild(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == nodeType {
			return c
		}
	}
	return nil
}

func findChildContent(node *sitter.Node, nodeType string, source []byte) string {
	c := findChild(node, nodeType)
	if c != nil {
		return c.Content(source)
	}
	return ""
}

func lastPathComponent(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
