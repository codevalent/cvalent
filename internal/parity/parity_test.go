package parity

import (
	"encoding/json"
	"testing"
)

func TestStripFields(t *testing.T) {
	input := map[string]interface{}{
		"name":        "foo",
		"id":          "some-uuid",
		"environment": "local",
		"nested": map[string]interface{}{
			"identity_source":     "distribution",
			"pipeline_references": []interface{}{},
			"keep":                42,
		},
		"items": []interface{}{
			map[string]interface{}{
				"id":            "x",
				"name":          "bar",
				"recent_traces": []interface{}{},
			},
		},
	}
	result := StripFields(input).(map[string]interface{})
	if _, has := result["id"]; has {
		t.Error("id should be stripped")
	}
	if _, has := result["environment"]; has {
		t.Error("environment should be stripped")
	}
	if result["name"] != "foo" {
		t.Error("name should be preserved")
	}
	nested := result["nested"].(map[string]interface{})
	if _, has := nested["identity_source"]; has {
		t.Error("nested identity_source should be stripped")
	}
	if nested["keep"] != 42 {
		t.Error("keep should be preserved")
	}
	items := result["items"].([]interface{})
	item := items[0].(map[string]interface{})
	if _, has := item["id"]; has {
		t.Error("item id should be stripped")
	}
	if item["name"] != "bar" {
		t.Error("item name should be preserved")
	}
}

func TestProjectAndMarshal(t *testing.T) {
	in := `{"id":"x","name":"foo","environment":"local"}`
	out, err := ProjectAndMarshal([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["id"]; has {
		t.Error("id should be stripped")
	}
	if m["name"] != "foo" {
		t.Error("name missing")
	}
}
