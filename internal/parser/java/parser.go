package java

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "java" }

func (p *Parser) Parse(run *parser.Run, file string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, java.GetLanguage())
	if err != nil {
		return nil, err
	}

	dist, err := run.Resolver.Resolve(file)
	if err != nil {
		return nil, err
	}
	pkg := extractPackage(tree, source)
	isTestFile := strings.HasSuffix(file, "Test.java") || strings.HasSuffix(file, "Tests.java")

	var funcs []parser.FunctionNode
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "class_declaration":
			funcs = append(funcs, extractClassMembers(dist, child, source, file, pkg, isTestFile)...)
		case "interface_declaration":
			funcs = append(funcs, extractInterfaceMembers(dist, child, source, file, pkg, isTestFile)...)
		}
	}
	// Disambiguate Java overloads after the fact: if any name+container
	// pair appears more than once, every occurrence in that group gets
	// re-minted with OverloadLanguage="java" so the sigHash suffix is
	// applied. This avoids paying the overhead for every method.
	disambiguateOverloads(funcs, dist, file, "java")
	return funcs, nil
}

// disambiguateOverloads re-mints any node whose (Container, Name) pair
// is non-unique within `funcs`, using OverloadLanguage to enable the
// sigHash suffix. The function mutates funcs in place.
func disambiguateOverloads(funcs []parser.FunctionNode, dist distresolver.Distribution, file, lang string) {
	type key struct{ recv, name string }
	counts := map[key]int{}
	for _, fn := range funcs {
		counts[key{fn.Receiver, fn.Name}]++
	}
	for i := range funcs {
		k := key{funcs[i].Receiver, funcs[i].Name}
		if counts[k] < 2 {
			continue
		}
		paramTypes := make([]string, len(funcs[i].Params))
		for j, p := range funcs[i].Params {
			if p.Type != "" {
				paramTypes[j] = p.Type
			} else {
				paramTypes[j] = p.TypeExpr
			}
		}
		container := strings.TrimPrefix(funcs[i].Receiver, "*")
		parts := model.IdentityParts{
			Distribution:     dist.Name,
			ModulePath:       funcs[i].ModulePath,
			Container:        container,
			Name:             funcs[i].Name,
			Params:           paramTypes,
			OverloadLanguage: lang,
		}
		base, err := parser.Mint(parts, lang, file, dist.Source)
		if err != nil {
			continue
		}
		funcs[i].Node = base
		funcs[i].Language = lang
	}
}

func extractPackage(tree *sitter.Node, source []byte) string {
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "package_declaration" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				c := child.NamedChild(j)
				if c.Type() == "scoped_identifier" || c.Type() == "identifier" {
					return c.Content(source)
				}
			}
		}
	}
	return ""
}

// classInfo holds field information for one-hop expansion.
type classInfo struct {
	name   string
	fields []parser.Field
}

func extractClassFields(classBody *sitter.Node, source []byte) map[string]classInfo {
	classes := map[string]classInfo{}
	// Walk the whole file for class declarations to find field types
	return classes
}

func extractFieldsFromBody(body *sitter.Node, source []byte) []parser.Field {
	var fields []parser.Field
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() != "field_declaration" {
			continue
		}
		typeStr := extractTypeStr(child, source)
		// Find variable declarators for field names
		for j := 0; j < int(child.NamedChildCount()); j++ {
			decl := child.NamedChild(j)
			if decl.Type() == "variable_declarator" {
				nameNode := decl.ChildByFieldName("name")
				if nameNode == nil {
					for k := 0; k < int(decl.NamedChildCount()); k++ {
						c := decl.NamedChild(k)
						if c.Type() == "identifier" {
							nameNode = c
							break
						}
					}
				}
				if nameNode != nil {
					fields = append(fields, parser.Field{
						Name: nameNode.Content(source),
						Type: typeStr,
					})
				}
			}
		}
	}
	return fields
}

func extractClassMembers(dist distresolver.Distribution, classNode *sitter.Node, source []byte, file, pkg string, isTestFile bool) []parser.FunctionNode {
	className := ""
	for i := 0; i < int(classNode.NamedChildCount()); i++ {
		c := classNode.NamedChild(i)
		if c.Type() == "identifier" {
			className = c.Content(source)
			break
		}
	}

	// Find class body and extract fields for one-hop expansion
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

	// Build same-file class map from fields
	classFields := extractFieldsFromBody(body, source)
	sameFileClasses := map[string]classInfo{
		className: {name: className, fields: classFields},
	}

	var funcs []parser.FunctionNode
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		switch child.Type() {
		case "method_declaration":
			fn, ok := extractMethod(dist, child, source, file, pkg, className, sameFileClasses, isTestFile)
			if ok {
				funcs = append(funcs, fn)
			}
		case "constructor_declaration":
			fn, ok := extractConstructor(dist, child, source, file, pkg, className, sameFileClasses)
			if ok {
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs
}

func extractInterfaceMembers(dist distresolver.Distribution, ifaceNode *sitter.Node, source []byte, file, pkg string, isTestFile bool) []parser.FunctionNode {
	ifaceName := ""
	for i := 0; i < int(ifaceNode.NamedChildCount()); i++ {
		c := ifaceNode.NamedChild(i)
		if c.Type() == "identifier" {
			ifaceName = c.Content(source)
			break
		}
	}

	var body *sitter.Node
	for i := 0; i < int(ifaceNode.NamedChildCount()); i++ {
		c := ifaceNode.NamedChild(i)
		if c.Type() == "interface_body" {
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
		if child.Type() == "method_declaration" {
			fn, ok := extractMethod(dist, child, source, file, pkg, ifaceName, nil, isTestFile)
			if ok {
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs
}

func extractMethod(dist distresolver.Distribution, node *sitter.Node, source []byte, file, pkg, className string, classes map[string]classInfo, isTestFile bool) (parser.FunctionNode, bool) {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" {
			name = c.Content(source)
			break
		}
	}

	modifiers := extractModifiers(node, source)
	annotations := extractAnnotations(node, source)
	isPublic := containsModifier(modifiers, "public")
	isStatic := containsModifier(modifiers, "static")

	returnType := extractReturnType(node, source, classes)
	params := extractParams(node, source, classes)

	tag := "application"
	if (isTestFile || containsAnnotation(annotations, "Test") || containsAnnotation(annotations, "ParameterizedTest")) &&
		(containsAnnotation(annotations, "Test") || containsAnnotation(annotations, "ParameterizedTest")) {
		tag = "test"
	}

	_ = isStatic
	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   pkg,
		Container:    className,
		Name:         name,
	}
	base, err := parser.Mint(parts, "java", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             isPublic,
			Tag:                  tag,
			Receiver:             className,
			Params:               params,
			Returns:              returnType,
			ContractCompleteness: "full",
		},
	}, true
}

func extractConstructor(dist distresolver.Distribution, node *sitter.Node, source []byte, file, pkg, className string, classes map[string]classInfo) (parser.FunctionNode, bool) {
	modifiers := extractModifiers(node, source)
	isPublic := containsModifier(modifiers, "public")
	params := extractConstructorParams(node, source, classes)

	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   pkg,
		Container:    className,
		Name:         className,
	}
	base, err := parser.Mint(parts, "java", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             isPublic,
			Tag:                  "application",
			Receiver:             className,
			Params:               params,
			Returns:              parser.ReturnSpec{Values: []parser.ReturnValue{}},
			ContractCompleteness: "full",
		},
	}, true
}

func extractModifiers(node *sitter.Node, source []byte) []string {
	var mods []string
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c.Type() == "modifiers" {
			// Use Child() not NamedChild() — keywords like "public" may be anonymous nodes
			for j := 0; j < int(c.ChildCount()); j++ {
				mod := c.Child(j)
				switch mod.Type() {
				case "public", "private", "protected", "static", "abstract", "final":
					mods = append(mods, mod.Type())
				}
			}
			break
		}
	}
	return mods
}

func extractAnnotations(node *sitter.Node, source []byte) []string {
	var anns []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "modifiers" {
			for j := 0; j < int(c.NamedChildCount()); j++ {
				mod := c.NamedChild(j)
				if mod.Type() == "marker_annotation" || mod.Type() == "annotation" {
					for k := 0; k < int(mod.NamedChildCount()); k++ {
						ann := mod.NamedChild(k)
						if ann.Type() == "identifier" {
							anns = append(anns, ann.Content(source))
						}
					}
				}
			}
			break
		}
	}
	return anns
}

func containsModifier(mods []string, target string) bool {
	for _, m := range mods {
		if m == target {
			return true
		}
	}
	return false
}

func containsAnnotation(anns []string, target string) bool {
	for _, a := range anns {
		if a == target {
			return true
		}
	}
	return false
}

func extractReturnType(node *sitter.Node, source []byte, classes map[string]classInfo) parser.ReturnSpec {
	// Find the return type node (comes after modifiers, before identifier)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		switch c.Type() {
		case "void_type":
			return parser.ReturnSpec{Values: []parser.ReturnValue{}}
		case "type_identifier", "generic_type", "array_type",
			"boolean_type", "integral_type", "floating_point_type":
			typeStr := c.Content(source)
			rv := parser.ReturnValue{Type: typeStr}

			// Check for nullable: Optional<T>
			if c.Type() == "generic_type" {
				baseName := ""
				for j := 0; j < int(c.NamedChildCount()); j++ {
					gc := c.NamedChild(j)
					if gc.Type() == "type_identifier" {
						baseName = gc.Content(source)
						break
					}
				}
				if baseName == "Optional" {
					rv.Nullable = true
				}
			}

			// One-hop expansion for same-file classes
			if classes != nil {
				baseType := typeStr
				if ci, ok := classes[baseType]; ok {
					rv.Expanded = ci.fields
				}
			}

			return parser.ReturnSpec{Values: []parser.ReturnValue{rv}}
		}
	}
	return parser.ReturnSpec{Values: []parser.ReturnValue{}}
}

func extractParams(node *sitter.Node, source []byte, classes map[string]classInfo) []parser.Param {
	params := []parser.Param{}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "formal_parameters" {
			for j := 0; j < int(c.NamedChildCount()); j++ {
				fp := c.NamedChild(j)
				if fp.Type() == "formal_parameter" || fp.Type() == "spread_parameter" {
					p := extractSingleParam(fp, source, classes)
					params = append(params, p)
				}
			}
			break
		}
	}
	return params
}

func extractConstructorParams(node *sitter.Node, source []byte, classes map[string]classInfo) []parser.Param {
	// Same as extractParams but for constructor_declaration
	return extractParams(node, source, classes)
}

func extractSingleParam(node *sitter.Node, source []byte, classes map[string]classInfo) parser.Param {
	// Check for annotations (e.g., @Nullable)
	annotations := extractParamAnnotations(node, source)
	nullable := containsAnnotation(annotations, "Nullable")

	typeStr := extractParamType(node, source)
	name := extractParamName(node, source)

	p := parser.Param{
		Name:     name,
		Type:     typeStr,
		Nullable: nullable,
	}

	// One-hop expansion for same-file classes
	if classes != nil {
		if ci, ok := classes[typeStr]; ok {
			p.Expanded = ci.fields
		}
	}

	return p
}

func extractParamAnnotations(node *sitter.Node, source []byte) []string {
	var anns []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "modifiers" {
			for j := 0; j < int(c.NamedChildCount()); j++ {
				mod := c.NamedChild(j)
				if mod.Type() == "marker_annotation" || mod.Type() == "annotation" {
					for k := 0; k < int(mod.NamedChildCount()); k++ {
						ann := mod.NamedChild(k)
						if ann.Type() == "identifier" {
							anns = append(anns, ann.Content(source))
						}
					}
				}
			}
		}
	}
	return anns
}

func extractParamType(node *sitter.Node, source []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		switch c.Type() {
		case "type_identifier", "generic_type", "array_type",
			"boolean_type", "integral_type", "floating_point_type", "void_type":
			return c.Content(source)
		}
	}
	return ""
}

func extractParamName(node *sitter.Node, source []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" {
			return c.Content(source)
		}
	}
	return ""
}

func extractTypeStr(node *sitter.Node, source []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		switch c.Type() {
		case "type_identifier", "generic_type", "array_type",
			"boolean_type", "integral_type", "floating_point_type", "void_type":
			return c.Content(source)
		}
	}
	return ""
}
