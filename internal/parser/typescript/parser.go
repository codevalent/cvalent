package typescript

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "typescript" }

func (p *Parser) Parse(run *parser.Run, file string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, typescript.GetLanguage())
	if err != nil {
		return nil, err
	}

	dist, err := run.Resolver.Resolve(file)
	if err != nil {
		return nil, err
	}
	modulePath := tsModulePath(file, dist.ManifestPath, run.Repo.Root)

	interfaces := extractInterfaces(tree, source)
	typeAliases := extractTypeAliases(tree, source)
	for k, v := range typeAliases {
		interfaces[k] = v
	}

	isTestFile := strings.HasSuffix(file, ".test.ts") ||
		strings.HasSuffix(file, ".spec.ts") ||
		strings.HasSuffix(file, ".test.tsx") ||
		strings.HasSuffix(file, ".spec.tsx")

	var funcs []parser.FunctionNode
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn, ok := extractFunction(dist, modulePath, child, source, file, "", interfaces, isTestFile, false)
			if ok {
				funcs = append(funcs, fn)
			}
		case "lexical_declaration":
			funcs = append(funcs, extractLexicalDeclaration(dist, modulePath, child, source, file, interfaces, isTestFile, false)...)
		case "export_statement":
			funcs = append(funcs, extractExportedItems(dist, modulePath, child, source, file, interfaces, isTestFile)...)
		}
	}

	disambiguateOverloads(funcs, dist, file)
	return funcs, nil
}

// tsModulePath converts a file path to a posix module path relative to
// the manifest dir, with the extension stripped (matching how TS imports
// resolve files).
func tsModulePath(file, manifestPath, repoRoot string) string {
	abs, _ := filepath.Abs(file)
	base := repoRoot
	if manifestPath != "" {
		base = filepath.Dir(manifestPath)
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, ext := range []string{".tsx", ".ts", ".jsx", ".js"} {
		if strings.HasSuffix(rel, ext) {
			rel = strings.TrimSuffix(rel, ext)
			break
		}
	}
	return rel
}

func disambiguateOverloads(funcs []parser.FunctionNode, dist distresolver.Distribution, file string) {
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
			OverloadLanguage: "typescript",
		}
		base, err := parser.Mint(parts, "typescript", file, dist.Source)
		if err != nil {
			continue
		}
		funcs[i].Node = base
	}
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

func extractExportedItems(dist distresolver.Distribution, modulePath string, node *sitter.Node, source []byte, file string, types map[string]typeInfo, isTestFile bool) []parser.FunctionNode {
	var funcs []parser.FunctionNode
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn, ok := extractFunction(dist, modulePath, child, source, file, "", types, isTestFile, true)
			if ok {
				funcs = append(funcs, fn)
			}
		case "lexical_declaration":
			funcs = append(funcs, extractLexicalDeclaration(dist, modulePath, child, source, file, types, isTestFile, true)...)
		case "class_declaration":
			funcs = append(funcs, extractClassMembers(dist, modulePath, child, source, file, types, isTestFile)...)
		}
	}
	return funcs
}

func extractFunction(dist distresolver.Distribution, modulePath string, node *sitter.Node, source []byte, file, container string, types map[string]typeInfo, isTestFile, exported bool) (parser.FunctionNode, bool) {
	name := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" {
			name = c.Content(source)
			break
		}
	}
	if name == "" {
		return parser.FunctionNode{}, false
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

	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   modulePath,
		Container:    container,
		Name:         name,
	}
	base, err := parser.Mint(parts, "typescript", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             exported,
			Tag:                  tag,
			Receiver:             container,
			Params:               params,
			Returns:              returns,
			ContractCompleteness: completeness,
		},
	}, true
}

func extractLexicalDeclaration(dist distresolver.Distribution, modulePath string, node *sitter.Node, source []byte, file string, types map[string]typeInfo, isTestFile, exported bool) []parser.FunctionNode {
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
				fn, ok := extractArrowFunction(dist, modulePath, arrowFn, name, source, file, types, isTestFile, exported)
				if ok {
					funcs = append(funcs, fn)
				}
			}
		}
	}
	return funcs
}

func extractArrowFunction(dist distresolver.Distribution, modulePath string, node *sitter.Node, name string, source []byte, file string, types map[string]typeInfo, isTestFile, exported bool) (parser.FunctionNode, bool) {
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

	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   modulePath,
		Name:         name,
	}
	base, err := parser.Mint(parts, "typescript", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             exported,
			Tag:                  tag,
			Params:               params,
			Returns:              returns,
			ContractCompleteness: completeness,
		},
	}, true
}

func extractClassMembers(dist distresolver.Distribution, modulePath string, classNode *sitter.Node, source []byte, file string, types map[string]typeInfo, isTestFile bool) []parser.FunctionNode {
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
			fn, ok := extractMethodDef(dist, modulePath, child, className, source, file, types, isTestFile)
			if ok {
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs
}

func extractMethodDef(dist distresolver.Distribution, modulePath string, node *sitter.Node, className string, source []byte, file string, types map[string]typeInfo, isTestFile bool) (parser.FunctionNode, bool) {
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

	if isConstructor {
		returns = parser.ReturnSpec{Values: []parser.ReturnValue{}}
	}

	tag := "application"
	if isTestFile {
		tag = "test"
	}
	completeness := "full"
	if hasAnyType(params, returns) {
		completeness = "partial"
	}

	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   modulePath,
		Container:    className,
		Name:         name,
	}
	base, err := parser.Mint(parts, "typescript", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             exported,
			Tag:                  tag,
			Receiver:             className,
			Params:               params,
			Returns:              returns,
			ContractCompleteness: completeness,
		},
	}, true
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
