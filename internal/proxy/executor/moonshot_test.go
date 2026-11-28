package executor

import (
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"
)

func moonshotTestBody(t *testing.T, model, fields string) map[string]any {
	t.Helper()
	out, err := NormalizeMoonshotRequest([]byte(`{"model":"`+model+`","messages":[{"role":"user","content":"hello"}]`+fields+`}`), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMoonshotK31Effort(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		want         any
	}{
		{"default", "", "max"},
		{"medium", `,"reasoning_effort":"medium"`, "high"},
		{"upper alias", `,"reasoning_effort":"XHIGH"`, "max"},
		{"light", `,"reasoning":{"effort":"light"}`, "low"},
		{"thinking effort wins disable", `,"thinking":{"type":"disabled","effort":"low"}`, "low"},
		{"explicit wins", `,"reasoning_effort":"high","reasoning":{"effort":"low"}`, "high"},
		{"none", `,"reasoning_effort":"none"`, "none"},
		{"off", `,"reasoning_effort":"off"`, "off"},
		{"disabled flag", `,"enable_thinking":false`, "none"},
		{"unknown", `,"reasoning_effort":"mystery"`, "mystery"},
		{"empty", `,"reasoning_effort":""`, ""},
		{"null explicit", `,"reasoning_effort":null,"reasoning":{"effort":"low"}`, nil},
		{"number", `,"reasoning_effort":123`, float64(123)},
		{"boolean", `,"reasoning_effort":false`, false},
		{"object", `,"reasoning_effort":{}`, map[string]any{}},
		{"array", `,"reasoning_effort":[]`, []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, model := range []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed"} {
				got := moonshotTestBody(t, model, tc.fields)
				if !reflect.DeepEqual(got["reasoning_effort"], tc.want) {
					t.Fatalf("%s effort = %#v, want %#v", model, got["reasoning_effort"], tc.want)
				}
				for _, key := range []string{"thinking", "reasoning", "enable_thinking"} {
					if _, leaked := got[key]; leaked {
						t.Errorf("%s leaked", key)
					}
				}
			}
		})
	}
}

func TestMoonshotFamilyBodyRules(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		ceiling     float64
		thinking    any
		choice      string
	}{
		{"k3", "kimi-k3", 1048576, nil, "required"},
		{"k3 alias", "k3-256k", 1048576, nil, "required"},
		{"k31", "kimi-k3.1-pro", 1048576, nil, "required"},
		{"k27", "kimi-k2.7-code", 262144, map[string]any{"type": "enabled", "keep": "all"}, "auto"},
		{"k27 highspeed", "kimi-k2.7-code-highspeed", 262144, map[string]any{"type": "enabled", "keep": "all"}, "auto"},
		{"k26", "kimi-k2.6", 262144, map[string]any{"type": "disabled"}, "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := moonshotTestBody(t, tc.model, `,"max_tokens":2000000,"temperature":0.3,"top_p":0.2,"frequency_penalty":1,"presence_penalty":1,"n":2,"logprobs":true,"top_logprobs":4,"enable_thinking":false,"tool_choice":"required"`)
			if got["max_completion_tokens"] != tc.ceiling || got["tool_choice"] != tc.choice || !reflect.DeepEqual(got["thinking"], tc.thinking) {
				t.Fatalf("body rules = %+v", got)
			}
			for _, key := range []string{"max_tokens", "temperature", "top_p", "frequency_penalty", "presence_penalty", "n", "logprobs", "top_logprobs", "enable_thinking"} {
				if _, leaked := got[key]; leaked {
					t.Errorf("unsupported %s leaked", key)
				}
			}
		})
	}
	got := moonshotTestBody(t, "kimi-k2.6", `,"reasoning_effort":"high","thinking":{"keep":"all"},"max_tokens":8,"max_completion_tokens":5`)
	if got["max_completion_tokens"] != float64(5) || !reflect.DeepEqual(got["thinking"], map[string]any{"type": "enabled", "keep": "all"}) {
		t.Fatalf("explicit token/keep precedence = %+v", got)
	}
}

func TestMoonshotToolsAndAuthenticReasoning(t *testing.T) {
	body := `{"model":"kimi-k3","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call1","type":"function","function":{"name":"search","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call1","content":"found"}],"tools":[{"type":"function","function":{"name":"search","parameters":{"type":"object","properties":{"root":{"type":"string"}},"required":["root"],"anyOf":[{"type":"object","properties":{"a":{"type":"string"},"shared":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["a","shared"]},{"type":"object","properties":{"b":{"type":"number"},"shared":{"type":"number"}},"required":["b","shared"]}]}}}]}`
	out, err := NormalizeMoonshotRequest([]byte(body), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	messages := payload["messages"].([]any)
	if _, fabricated := messages[0].(map[string]any)["reasoning_content"]; fabricated {
		t.Fatal("fabricated K3 reasoning")
	}
	if messages[1].(map[string]any)["name"] != "search" {
		t.Fatal("tool result name not repaired")
	}
	parameters := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if _, remaining := parameters["anyOf"]; remaining {
		t.Fatal("root union not flattened")
	}
	if !reflect.DeepEqual(parameters["required"], []any{"root", "shared"}) {
		t.Fatalf("requirements = %+v", parameters["required"])
	}
	shared := parameters["properties"].(map[string]any)["shared"].(map[string]any)
	if _, kept := shared["anyOf"]; !kept {
		t.Fatal("nested union or first property definition lost")
	}
	if payload["stream_options"].(map[string]any)["include_usage"] != true {
		t.Fatal("stream usage not requested")
	}
	for _, model := range []string{"kimi-k3.1-pro", "kimi-k2.7-code"} {
		out, err := NormalizeMoonshotRequest([]byte(strings.Replace(body, "kimi-k3", model, 1)), false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), `"reasoning_content":" "`) {
			t.Fatalf("fabricated %s reasoning", model)
		}
	}
	for _, schema := range []string{`{"anyOf":[{"$ref":"#/$defs/a"},{"type":"object"}]}`, `{"anyOf":[{"type":"string"},{"type":"object"}]}`} {
		got := moonshotTestBody(t, "kimi-k3", `,"tools":[{"type":"function","function":{"name":"search","parameters":`+schema+`}}]`)
		parameters := got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
		if _, kept := parameters["anyOf"]; !kept {
			t.Fatal("incompatible root union flattened")
		}
	}
}

func TestMoonshotStreamOptionsAndReasoning(t *testing.T) {
	body := []byte(`{"model":"moonshot/kimi-k2.6","stream_options":{"include_usage":true},"messages":[{"role":"assistant","content":"ok"},{"role":"assistant","content":"ok","reasoning":"real reasoning"}]}`)
	out, err := NormalizeMoonshotRequest(body, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "stream_options") || !strings.Contains(string(out), `"reasoning_content":" "`) || !strings.Contains(string(out), `"reasoning":"real reasoning"`) {
		t.Fatalf("legacy body = %s", out)
	}
	out, err = NormalizeMoonshotRequest(body, true, map[string]any{"disableStreamOptions": true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "stream_options") || !strings.Contains(string(out), `"stream":true`) {
		t.Fatalf("stream opt-out = %s", out)
	}
}
