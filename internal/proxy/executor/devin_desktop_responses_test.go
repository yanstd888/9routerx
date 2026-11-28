package executor

import (
	"bytes"
	json "encoding/json/v2"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/translator"
)

func TestDevinDesktopResponsesCompletedItems(t *testing.T) {
	client, _, _ := devinDesktopTestClient(t, devinDesktopTestChat(t))
	req := devinDesktopTestRequest(client, []byte(`{"input":"hi"}`))
	req.Ctx = translator.WithClientFormat(req.Ctx, translator.ClientFormatResponses)
	req.IsStream = true
	w := httptest.NewRecorder()
	if err := ForwardDevinDesktop(w, req); err != nil {
		t.Fatal(err)
	}
	indexes := map[string]int{}
	var output []struct {
		Type string `json:"type"`
	}
	for _, line := range bytes.Split(w.Body.Bytes(), []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event struct {
			Type  string `json:"type"`
			Index int    `json:"output_index"`
			Item  struct {
				ID string `json:"id"`
			} `json:"item"`
			Response struct {
				Output []struct {
					Type string `json:"type"`
				} `json:"output"`
			} `json:"response"`
		}
		if err := json.Unmarshal(line[6:], &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "response.output_item.added" {
			indexes[event.Item.ID] = event.Index
		}
		if event.Type == "response.completed" {
			output = event.Response.Output
		}
	}
	if len(indexes) != 3 || len(output) != 3 {
		t.Fatalf("indexes=%v output=%v", indexes, output)
	}
	seen := map[int]bool{}
	for _, index := range indexes {
		if seen[index] {
			t.Errorf("duplicate output index %d", index)
		}
		seen[index] = true
	}
	for i, want := range []string{"reasoning", "message", "function_call"} {
		if output[i].Type != want {
			t.Errorf("output[%d]=%s", i, output[i].Type)
		}
	}
}

func TestDevinDesktopResponsesCustomTool(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			call := append(devinDesktopTestString(1, "custom1"), devinDesktopTestString(2, "run")...)
			call = append(call, devinDesktopTestString(3, `{"input":"print(1)"}`)...)
			chat := append(devinDesktopTestFrame(devinDesktopTestString(6, string(call)), 0), devinDesktopTestFrame([]byte(`{}`), 2)...)
			client, _, _ := devinDesktopTestClient(t, chat)
			req := devinDesktopTestRequest(client, []byte(`{"input":"hi","tools":[{"type":"custom","name":"run","description":"Run code","format":{"type":"text"}}]}`))
			req.Ctx = translator.WithClientFormat(req.Ctx, translator.ClientFormatResponses)
			req.IsStream = stream
			w := httptest.NewRecorder()
			if err := ForwardDevinDesktop(w, req); err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{"custom_tool_call", "print(1)"} {
				if !strings.Contains(w.Body.String(), part) {
					t.Errorf("missing %q in %s", part, w.Body.String())
				}
			}
		})
	}
}
