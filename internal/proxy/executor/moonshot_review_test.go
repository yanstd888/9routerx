package executor

import (
	"reflect"
	"testing"
)

func TestMoonshotReviewSchemaHygiene(t *testing.T) {
	got := moonshotTestBody(t, "kimi-k3.1-pro", `,"tools":[{"type":"function","function":{"name":"f","parameters":{"type":null,"properties":{"choice":{"anyOf":[{"type":"string","enum":["a",null]},{"type":"null"}]},"tuple":{"type":"array","items":[false,{"type":"string","enum":["b",null]}]}}}}}]`)
	p := got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if p["type"] != "object" {
		t.Errorf("root type:null not normalized: %+v", p)
	}
	props := p["properties"].(map[string]any)
	union := props["choice"].(map[string]any)["anyOf"].([]any)
	if !reflect.DeepEqual(union[0].(map[string]any)["enum"], []any{"a"}) {
		t.Errorf("nested nullable enum retained: %+v", union)
	}
	items, ok := props["tuple"].(map[string]any)["items"].(map[string]any)
	if !ok || !reflect.DeepEqual(items["enum"], []any{"b"}) {
		t.Errorf("tuple items not normalized: %+v", props["tuple"])
	}
}

func TestMoonshotReviewRequiredWithoutProperties(t *testing.T) {
	for _, tc := range []struct {
		name, schema string
		want         any
	}{
		{"absent", `{"anyOf":[{"required":["x"]},{"required":["x"]}]}`, []any{"x"}},
		{"empty", `{"properties":{},"anyOf":[{"required":["x"]},{"required":["x"]}]}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := moonshotTestBody(t, "kimi-k3", `,"tools":[{"type":"function","function":{"name":"f","parameters":`+tc.schema+`}}]`)
			p := got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
			if !reflect.DeepEqual(p["required"], tc.want) {
				t.Fatalf("required = %#v, want %#v", p["required"], tc.want)
			}
		})
	}
}
