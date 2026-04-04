package java

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/codevalent/cvalent/internal/parser"
)

func TestJavaParser_Fixtures(t *testing.T) {
	fixtures := []struct {
		name  string
		dir   string
		files []string
	}{
		{"basic", "../../../testdata/java/basic", []string{"input/OrderService.java"}},
		{"inheritance", "../../../testdata/java/inheritance", []string{"input/Processor.java"}},
		{"nullable", "../../../testdata/java/nullable", []string{"input/UserService.java"}},
		{"generics", "../../../testdata/java/generics", []string{"input/Repository.java"}},
		{"test_tagging", "../../../testdata/java/test_tagging", []string{"input/OrderServiceTest.java"}},
		{"static_constructors", "../../../testdata/java/static_constructors", []string{"input/Config.java"}},
	}

	p := New()
	if p.Language() != "java" {
		t.Fatalf("expected language 'java', got %q", p.Language())
	}

	for _, fix := range fixtures {
		t.Run(fix.name, func(t *testing.T) {
			expectedPath := filepath.Join(fix.dir, "expected.json")
			expectedData, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read expected.json: %v", err)
			}
			var expected []parser.FunctionNode
			if err := json.Unmarshal(expectedData, &expected); err != nil {
				t.Fatalf("parse expected.json: %v", err)
			}

			var actual []parser.FunctionNode
			for _, file := range fix.files {
				fullPath := filepath.Join(fix.dir, file)
				source, err := os.ReadFile(fullPath)
				if err != nil {
					t.Fatalf("read %s: %v", file, err)
				}
				nodes, err := p.Parse(file, source)
				if err != nil {
					t.Fatalf("parse %s: %v", file, err)
				}
				actual = append(actual, nodes...)
			}

			if len(actual) != len(expected) {
				actualJSON, _ := json.MarshalIndent(actual, "", "  ")
				t.Fatalf("count: expected %d, got %d\nActual:\n%s",
					len(expected), len(actual), string(actualJSON))
			}

			for i := range expected {
				assertFnEqual(t, expected[i], actual[i], i)
			}
		})
	}
}

func assertFnEqual(t *testing.T, exp, act parser.FunctionNode, idx int) {
	t.Helper()
	check := func(field, e, a string) {
		if e != a {
			t.Errorf("fn[%d] %s: expected %q, got %q", idx, field, e, a)
		}
	}
	check("name", exp.Name, act.Name)
	check("qualified_name", exp.QualifiedName, act.QualifiedName)
	check("file", exp.File, act.File)
	check("language", exp.Language, act.Language)
	check("kind", exp.Kind, act.Kind)
	check("receiver", exp.Receiver, act.Receiver)
	check("tag", exp.Tag, act.Tag)
	check("contract_completeness", exp.ContractCompleteness, act.ContractCompleteness)

	if exp.StartLine != act.StartLine {
		t.Errorf("fn[%d] start_line: expected %d, got %d", idx, exp.StartLine, act.StartLine)
	}
	if exp.EndLine != act.EndLine {
		t.Errorf("fn[%d] end_line: expected %d, got %d", idx, exp.EndLine, act.EndLine)
	}
	if exp.Exported != act.Exported {
		t.Errorf("fn[%d] exported: expected %v, got %v", idx, exp.Exported, act.Exported)
	}

	if len(exp.Parameters) != len(act.Parameters) {
		aJSON, _ := json.MarshalIndent(act.Parameters, "", "  ")
		t.Errorf("fn[%d] params: expected %d, got %d\nActual: %s", idx, len(exp.Parameters), len(act.Parameters), aJSON)
	} else {
		for i := range exp.Parameters {
			ep, ap := exp.Parameters[i], act.Parameters[i]
			if ep.Name != ap.Name || ep.Type != ap.Type || ep.Nullable != ap.Nullable {
				t.Errorf("fn[%d] param[%d]: expected {%s %s nullable=%v}, got {%s %s nullable=%v}",
					idx, i, ep.Name, ep.Type, ep.Nullable, ap.Name, ap.Type, ap.Nullable)
			}
		}
	}

	if len(exp.Returns.Values) != len(act.Returns.Values) {
		aJSON, _ := json.MarshalIndent(act.Returns, "", "  ")
		t.Errorf("fn[%d] returns: expected %d, got %d\nActual: %s", idx, len(exp.Returns.Values), len(act.Returns.Values), aJSON)
	} else {
		for i := range exp.Returns.Values {
			er, ar := exp.Returns.Values[i], act.Returns.Values[i]
			if er.Type != ar.Type || er.Nullable != ar.Nullable {
				t.Errorf("fn[%d] return[%d]: expected {%s nullable=%v}, got {%s nullable=%v}",
					idx, i, er.Type, er.Nullable, ar.Type, ar.Nullable)
			}
		}
	}
}
