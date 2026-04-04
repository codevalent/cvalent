package golang

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/parser"
)

func TestGoParser_Fixtures(t *testing.T) {
	fixtures := []struct {
		name    string
		dir     string
		files   []string // relative to fixture dir
	}{
		{"basic", "../../../testdata/go/basic", []string{"input/main.go"}},
		{"nullable", "../../../testdata/go/nullable", []string{"input/service.go"}},
		{"methods", "../../../testdata/go/methods", []string{"input/types.go"}},
		{"test_tagging", "../../../testdata/go/test_tagging", []string{"input/handler_test.go"}},
		{"multiple_returns", "../../../testdata/go/multiple_returns", []string{"input/multi.go"}},
		{"edge_cases", "../../../testdata/go/edge_cases", []string{"input/edge.go"}},
	}

	p := New()
	if p.Language() != "go" {
		t.Fatalf("expected language 'go', got %q", p.Language())
	}

	for _, fix := range fixtures {
		t.Run(fix.name, func(t *testing.T) {
			// Load expected output
			expectedPath := filepath.Join(fix.dir, "expected.json")
			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("failed to read expected.json: %v", err)
			}
			var expected []parser.FunctionNode
			if err := json.Unmarshal(expectedData, &expected); err != nil {
				t.Fatalf("failed to parse expected.json: %v", err)
			}

			// Parse all input files
			var actual []parser.FunctionNode
			for _, file := range fix.files {
				fullPath := filepath.Join(fix.dir, file)
				source, err := os.ReadFile(fullPath)
				if err != nil {
					t.Fatalf("failed to read %s: %v", file, err)
				}
				nodes, err := p.Parse(file, source)
				if err != nil {
					t.Fatalf("parse error for %s: %v", file, err)
				}
				actual = append(actual, nodes...)
			}

			if len(actual) != len(expected) {
				actualJSON, _ := json.MarshalIndent(actual, "", "  ")
				t.Fatalf("expected %d functions, got %d.\nActual:\n%s",
					len(expected), len(actual), string(actualJSON))
			}

			for i := range expected {
				assertFunctionEqual(t, expected[i], actual[i], i)
			}
		})
	}
}

func assertFunctionEqual(t *testing.T, expected, actual parser.FunctionNode, idx int) {
	t.Helper()

	if actual.Name != expected.Name {
		t.Errorf("fn[%d] name: expected %q, got %q", idx, expected.Name, actual.Name)
	}
	if actual.QualifiedName != expected.QualifiedName {
		t.Errorf("fn[%d] qualified_name: expected %q, got %q", idx, expected.QualifiedName, actual.QualifiedName)
	}
	if actual.File != expected.File {
		t.Errorf("fn[%d] file: expected %q, got %q", idx, expected.File, actual.File)
	}
	if actual.Package != expected.Package {
		t.Errorf("fn[%d] package: expected %q, got %q", idx, expected.Package, actual.Package)
	}
	if actual.Language != expected.Language {
		t.Errorf("fn[%d] language: expected %q, got %q", idx, expected.Language, actual.Language)
	}
	if actual.StartLine != expected.StartLine {
		t.Errorf("fn[%d] start_line: expected %d, got %d", idx, expected.StartLine, actual.StartLine)
	}
	if actual.EndLine != expected.EndLine {
		t.Errorf("fn[%d] end_line: expected %d, got %d", idx, expected.EndLine, actual.EndLine)
	}
	if actual.Kind != expected.Kind {
		t.Errorf("fn[%d] kind: expected %q, got %q", idx, expected.Kind, actual.Kind)
	}
	if actual.Receiver != expected.Receiver {
		t.Errorf("fn[%d] receiver: expected %q, got %q", idx, expected.Receiver, actual.Receiver)
	}
	if actual.Exported != expected.Exported {
		t.Errorf("fn[%d] exported: expected %v, got %v", idx, expected.Exported, actual.Exported)
	}
	if actual.Tag != expected.Tag {
		t.Errorf("fn[%d] tag: expected %q, got %q", idx, expected.Tag, actual.Tag)
	}
	if actual.ContractCompleteness != expected.ContractCompleteness {
		t.Errorf("fn[%d] contract_completeness: expected %q, got %q", idx, expected.ContractCompleteness, actual.ContractCompleteness)
	}

	// Compare parameters
	assertParamsEqual(t, expected.Parameters, actual.Parameters, idx)

	// Compare returns
	assertReturnsEqual(t, expected.Returns, actual.Returns, idx)
}

func assertParamsEqual(t *testing.T, expected, actual []parser.Param, fnIdx int) {
	t.Helper()
	if len(actual) != len(expected) {
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Errorf("fn[%d] params count: expected %d, got %d\nExpected: %s\nActual: %s",
			fnIdx, len(expected), len(actual), string(expectedJSON), string(actualJSON))
		return
	}
	for i := range expected {
		if actual[i].Name != expected[i].Name {
			t.Errorf("fn[%d] param[%d] name: expected %q, got %q", fnIdx, i, expected[i].Name, actual[i].Name)
		}
		if actual[i].Type != expected[i].Type {
			t.Errorf("fn[%d] param[%d] type: expected %q, got %q", fnIdx, i, expected[i].Type, actual[i].Type)
		}
		if actual[i].TypeExpr != expected[i].TypeExpr {
			t.Errorf("fn[%d] param[%d] type_expr: expected %q, got %q", fnIdx, i, expected[i].TypeExpr, actual[i].TypeExpr)
		}
		if actual[i].Nullable != expected[i].Nullable {
			t.Errorf("fn[%d] param[%d] nullable: expected %v, got %v", fnIdx, i, expected[i].Nullable, actual[i].Nullable)
		}
		if actual[i].Variadic != expected[i].Variadic {
			t.Errorf("fn[%d] param[%d] variadic: expected %v, got %v", fnIdx, i, expected[i].Variadic, actual[i].Variadic)
		}
		assertFieldsEqual(t, expected[i].Expanded, actual[i].Expanded, fnIdx, i, "param")
	}
}

func assertReturnsEqual(t *testing.T, expected, actual parser.ReturnSpec, fnIdx int) {
	t.Helper()
	if actual.Multiple != expected.Multiple {
		t.Errorf("fn[%d] returns.multiple: expected %v, got %v", fnIdx, expected.Multiple, actual.Multiple)
	}
	if len(actual.Values) != len(expected.Values) {
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Errorf("fn[%d] returns count: expected %d, got %d\nExpected: %s\nActual: %s",
			fnIdx, len(expected.Values), len(actual.Values), string(expectedJSON), string(actualJSON))
		return
	}
	for i := range expected.Values {
		if actual.Values[i].Name != expected.Values[i].Name {
			t.Errorf("fn[%d] return[%d] name: expected %q, got %q", fnIdx, i, expected.Values[i].Name, actual.Values[i].Name)
		}
		if actual.Values[i].Type != expected.Values[i].Type {
			t.Errorf("fn[%d] return[%d] type: expected %q, got %q", fnIdx, i, expected.Values[i].Type, actual.Values[i].Type)
		}
		if actual.Values[i].Nullable != expected.Values[i].Nullable {
			t.Errorf("fn[%d] return[%d] nullable: expected %v, got %v", fnIdx, i, expected.Values[i].Nullable, actual.Values[i].Nullable)
		}
		assertFieldsEqual(t, expected.Values[i].Expanded, actual.Values[i].Expanded, fnIdx, i, "return")
	}
}

func assertFieldsEqual(t *testing.T, expected, actual []parser.Field, fnIdx, elemIdx int, context string) {
	t.Helper()
	if len(actual) != len(expected) {
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Errorf("fn[%d] %s[%d] expanded count: expected %d, got %d\nExpected: %s\nActual: %s",
			fnIdx, context, elemIdx, len(expected), len(actual), string(expectedJSON), string(actualJSON))
		return
	}
	for i := range expected {
		if actual[i].Name != expected[i].Name {
			t.Errorf("fn[%d] %s[%d] field[%d] name: expected %q, got %q", fnIdx, context, elemIdx, i, expected[i].Name, actual[i].Name)
		}
		if actual[i].Type != expected[i].Type {
			t.Errorf("fn[%d] %s[%d] field[%d] type: expected %q, got %q", fnIdx, context, elemIdx, i, expected[i].Type, actual[i].Type)
		}
		if actual[i].Nullable != expected[i].Nullable {
			t.Errorf("fn[%d] %s[%d] field[%d] nullable: expected %v, got %v", fnIdx, context, elemIdx, i, expected[i].Nullable, actual[i].Nullable)
		}
	}
}
