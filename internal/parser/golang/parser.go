// Package golang is the Go function-extractor parser.
//
// Identity at Rung 0 follows docs/identity-spec.md:
//
//   - Distribution comes from go.mod via internal/parser/distresolver.
//   - Module path is the package import path inside the distribution
//     (e.g. "internal/widget"), computed from the file's directory
//     relative to the directory containing go.mod.
//   - Container for methods is the receiver type without any pointer
//     prefix; pointer-vs-value is carried in PointerReceiver and the
//     canonicalizer wraps as "(*T)" if set.
//   - Generics on the receiver type ("Box[T]") and on the function name
//     ("Map[T any]") are stripped by Canonicalize.
//
// Identities are minted only via parser.Mint.
package golang

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/codevalent/cvalent/internal/model"
	"github.com/codevalent/cvalent/internal/parser"
	"github.com/codevalent/cvalent/internal/parser/distresolver"
)

// Parser extracts function nodes from Go source files using tree-sitter.
type Parser struct{}

func New() *Parser { return &Parser{} }

func (p *Parser) Language() string { return "go" }

func (p *Parser) Parse(run *parser.Run, file string, source []byte) ([]parser.FunctionNode, error) {
	tree, err := sitter.ParseCtx(context.Background(), source, golang.GetLanguage())
	if err != nil {
		return nil, err
	}

	dist, err := run.Resolver.Resolve(file)
	if err != nil {
		return nil, err
	}
	modulePath := goModulePath(file, dist.ManifestPath, run.Repo.Root)

	structs := extractStructs(tree, source)
	isTestFile := strings.HasSuffix(file, "_test.go")

	var funcs []parser.FunctionNode
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			fn, ok, err := buildFunction(dist, modulePath, file, child, source, structs, isTestFile)
			if err != nil {
				return nil, err
			}
			if ok {
				funcs = append(funcs, fn)
			}
		case "method_declaration":
			fn, ok, err := buildMethod(dist, modulePath, file, child, source, structs, isTestFile)
			if err != nil {
				return nil, err
			}
			if ok {
				funcs = append(funcs, fn)
			}
		}
	}
	return funcs, nil
}

// goModulePath computes the in-module package path for `file` given the
// resolver's manifest path (path to go.mod) and the repo root. Result is
// in slash form, e.g. "internal/widget".
func goModulePath(file, manifestPath, repoRoot string) string {
	absFile, _ := filepath.Abs(file)
	dir := filepath.Dir(absFile)
	moduleDir := repoRoot
	if manifestPath != "" {
		moduleDir = filepath.Dir(manifestPath)
	}
	rel, err := filepath.Rel(moduleDir, dir)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func buildFunction(dist distresolver.Distribution, modulePath, file string, node *sitter.Node, source []byte, structs map[string]structInfo, isTestFile bool) (parser.FunctionNode, bool, error) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parser.FunctionNode{}, false, nil
	}
	name := nameNode.Content(source)

	params := extractParams(node.ChildByFieldName("parameters"), source, structs)
	returns := extractReturns(node.ChildByFieldName("result"), source, structs)

	parts := model.IdentityParts{
		Distribution: dist.Name,
		ModulePath:   modulePath,
		Name:         name,
	}
	base, err := parser.Mint(parts, "go", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false, err
	}
	tag := "application"
	if isTestFile && isTestFuncName(name) {
		tag = "test"
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             isExported(name),
			Tag:                  tag,
			Params:               params,
			Returns:              returns,
			ContractCompleteness: "full",
		},
	}, true, nil
}

func buildMethod(dist distresolver.Distribution, modulePath, file string, node *sitter.Node, source []byte, structs map[string]structInfo, isTestFile bool) (parser.FunctionNode, bool, error) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return parser.FunctionNode{}, false, nil
	}
	name := nameNode.Content(source)
	receiverType, pointer := extractReceiver(node, source)

	params := extractParams(node.ChildByFieldName("parameters"), source, structs)
	returns := extractReturns(node.ChildByFieldName("result"), source, structs)

	parts := model.IdentityParts{
		Distribution:    dist.Name,
		ModulePath:      modulePath,
		Container:       receiverType,
		PointerReceiver: pointer,
		Name:            name,
	}
	base, err := parser.Mint(parts, "go", file, dist.Source)
	if err != nil {
		return parser.FunctionNode{}, false, err
	}
	tag := "application"
	if isTestFile && isTestFuncName(name) {
		tag = "test"
	}
	receiverDisplay := receiverType
	if pointer {
		receiverDisplay = "*" + receiverType
	}
	return parser.FunctionNode{
		Node: base,
		FunctionMeta: model.FunctionMeta{
			StartLine:            int(node.StartPoint().Row) + 1,
			EndLine:              int(node.EndPoint().Row) + 1,
			Exported:             isExported(name),
			Tag:                  tag,
			Receiver:             receiverDisplay,
			PointerReceiver:      pointer,
			Params:               params,
			Returns:              returns,
			ContractCompleteness: "full",
		},
	}, true, nil
}

func extractPackage(tree *sitter.Node, source []byte) string {
	for i := 0; i < int(tree.NamedChildCount()); i++ {
		child := tree.NamedChild(i)
		if child.Type() == "package_clause" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(source)
			}
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
	fieldList := structNode.ChildByFieldName("body")
	if fieldList == nil {
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

// extractReceiver returns the receiver type without a pointer prefix and
// a bool indicating pointer-receiver.
func extractReceiver(node *sitter.Node, source []byte) (string, bool) {
	receiverNode := node.ChildByFieldName("receiver")
	if receiverNode == nil {
		return "", false
	}
	for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
		param := receiverNode.NamedChild(i)
		if param.Type() == "parameter_declaration" {
			typeNode := param.ChildByFieldName("type")
			if typeNode == nil {
				continue
			}
			raw := typeNode.Content(source)
			if strings.HasPrefix(raw, "*") {
				return strings.TrimPrefix(raw, "*"), true
			}
			return raw, false
		}
	}
	return "", false
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
	return parser.Param{Name: name, Type: typeStr, Variadic: true}
}

func extractReturns(resultNode *sitter.Node, source []byte, structs map[string]structInfo) parser.ReturnSpec {
	if resultNode == nil {
		return parser.ReturnSpec{Values: []parser.ReturnValue{}}
	}
	switch resultNode.Type() {
	case "parameter_list":
		var values []parser.ReturnValue
		for i := 0; i < int(resultNode.NamedChildCount()); i++ {
			child := resultNode.NamedChild(i)
			if child.Type() == "parameter_declaration" {
				rv := extractReturnValue(child, source, structs)
				values = append(values, rv)
			}
		}
		return parser.ReturnSpec{Multiple: len(values) > 1, Values: values}
	default:
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

func shouldUseTypeExpr(typeStr string, structs map[string]structInfo) bool {
	base := typeStr
	for strings.HasPrefix(base, "*") || strings.HasPrefix(base, "[]") {
		base = strings.TrimPrefix(base, "*")
		base = strings.TrimPrefix(base, "[]")
	}
	if isPrimitive(base) {
		return false
	}
	if strings.Contains(base, ".") {
		return false
	}
	if base == "interface{}" {
		return false
	}
	if _, ok := structs[base]; ok {
		return false
	}
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
