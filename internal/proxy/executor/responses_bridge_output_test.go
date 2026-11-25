package executor

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/translator"
)

const bridgeOutputUsage = `"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":2}}`

func bridgeOutputChunks(secondArgs string) []string {
	return []string{
		`{"id":"c16","choices":[{"index":0,"delta":{"reasoning_content":"Check "}}]}`,
		`{"id":"c16","choices":[{"index":0,"delta":{"reasoning_content":"both tools."}}]}`,
		`{"id":"c16","choices":[{"index":0,"delta":{"content":"Use "}}]}`,
		`{"id":"c16","choices":[{"index":0,"delta":{"content":"both."}}]}`,
		`{"id":"c16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_16_a","type":"function","function":{"name":"first_tool","arguments":"{\"a\":"}},{"index":1,"id":"call_16_b","type":"function","function":{"name":"second_tool","arguments":""}}]}}]}`,
		`{"id":"c16","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}},{"index":1,"function":{"arguments":` + jsonString(secondArgs) + `}}]}}]}`,
	}
}

func TestResponsesBridgeTerminalOutput_ReasoningMessageTwoDistinctTools(t *testing.T) {
	paths := []string{"finish_with_usage", "usage_trailer_done", "close_pending", "close_open", "watchdog"}
	for _, incomplete := range []bool{false, true} {
		status, secondArgs := "completed", `{"b":2}`
		if incomplete {
			status, secondArgs = "incomplete", `{"b":`
		}
		for _, path := range paths {
			t.Run(status+"/"+path, func(t *testing.T) {
				withFlushTimeout(t, 40*time.Millisecond)
				frames := newFrameCollector()
				bridge := NewResponsesBridge("fixture-model", nil, frames.write)
				t.Cleanup(bridge.Close)
				reference := translator.InitResponsesState("fixture-model", true)
				reference.Created = bridge.state.Created
				reference.ResponseID = bridge.state.ResponseID
				var original strings.Builder
				feed := func(payload string) {
					bridge.Feed([]byte(payload))
					for _, ev := range translator.TranslateOpenAIToResponses(parseChatChunk([]byte(payload)), reference) {
						original.WriteString(translator.FormatResponsesSSE(ev))
					}
				}
				for _, payload := range bridgeOutputChunks(secondArgs) {
					feed(payload)
				}
				finish := `{"id":"c16","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
				switch path {
				case "finish_with_usage":
					feed(strings.TrimSuffix(finish, "}") + "," + bridgeOutputUsage + "}")
				case "usage_trailer_done":
					feed(finish)
					feed(`{"id":"c16","choices":[],` + bridgeOutputUsage + `}`)
					feed("[DONE]")
				case "close_pending", "watchdog":
					feed(finish)
					if !bridge.CompletionPending() {
						t.Fatal("finish without usage must leave the terminal pending")
					}
				}
				if path == "watchdog" {
					waitBridgeOutputTerminal(t, frames)
					bridge.mu.Lock()
					finished := bridge.finished
					bridge.mu.Unlock()
					if !finished {
						t.Fatal("watchdog did not finish the bridge before Close")
					}
				}
				bridge.Close()
				bridge.Close()
				bridge.Feed([]byte(finish))
				for _, ev := range translator.FlushResponses(reference) {
					original.WriteString(translator.FormatResponsesSSE(ev))
				}
				if err := bridge.Err(); err != nil {
					t.Fatalf("bridge write: %v", err)
				}
				assertBridgeOutputTerminal(t, frames.text(), status, secondArgs)
				assertBridgeOutputOnlyTerminalChanged(t, frames.text(), original.String())
			})
		}
	}
}

func waitBridgeOutputTerminal(t *testing.T, frames *frameCollector) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for !strings.Contains(frames.text(), `"type":"response.completed"`) && !strings.Contains(frames.text(), `"type":"response.incomplete"`) {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("watchdog never emitted the pending terminal")
		}
	}
}

func bridgeOutputData(t *testing.T, frame string) map[string]any {
	t.Helper()
	_, data, ok := strings.Cut(frame, "\ndata: ")
	if !ok {
		t.Fatalf("not a Responses SSE frame: %q", frame)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("decode Responses event: %v", err)
	}
	return result
}

func assertBridgeOutputTerminal(t *testing.T, body, status, secondArgs string) {
	t.Helper()
	var terminal map[string]any
	count := 0
	for _, frame := range strings.Split(strings.TrimSpace(body), "\n\n") {
		data := bridgeOutputData(t, frame)
		if data["type"] == "response.completed" || data["type"] == "response.incomplete" {
			count++
			if data["type"] != "response."+status {
				t.Errorf("terminal type = %v, want response.%s", data["type"], status)
			}
			terminal = data["response"].(map[string]any)
		}
	}
	if count != 1 {
		t.Fatalf("terminal count = %d, want exactly 1", count)
	}
	if terminal["status"] != status {
		t.Errorf("terminal status = %v, want %s", terminal["status"], status)
	}
	if status == "incomplete" {
		want := map[string]any{"reason": "tool_call_arguments_incomplete"}
		if !reflect.DeepEqual(terminal["incomplete_details"], want) {
			t.Errorf("incomplete details = %v, want %v", terminal["incomplete_details"], want)
		}
	}
	output, ok := terminal["output"].([]any)
	if !ok || len(output) != 4 {
		t.Fatalf("terminal lost item identities: output = %v, want reasoning, message, and two distinct tools at positions 0-3", terminal["output"])
	}
	want := []any{
		map[string]any{"id": "rs_resp_c16_0", "type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "Check both tools."}}},
		map[string]any{"id": "msg_resp_c16_0", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Use both.", "annotations": []any{}, "logprobs": []any{}}}},
		map[string]any{"id": "fc_call_16_a", "type": "function_call", "call_id": "call_16_a", "name": "first_tool", "arguments": `{"a":1}`},
		map[string]any{"id": "fc_call_16_b", "type": "function_call", "call_id": "call_16_b", "name": "second_tool", "arguments": secondArgs},
	}
	if !reflect.DeepEqual(output, want) {
		t.Errorf("terminal positions 0-3 must contain complete final snapshots without invented fields:\ngot  %#v\nwant %#v", output, want)
	}
}

func assertBridgeOutputOnlyTerminalChanged(t *testing.T, body, original string) {
	t.Helper()
	gotFrames := strings.Split(strings.TrimSpace(body), "\n\n")
	wantFrames := strings.Split(strings.TrimSpace(original), "\n\n")
	if len(gotFrames) != len(wantFrames) {
		t.Fatalf("wire event count = %d, unchanged translator emitted %d", len(gotFrames), len(wantFrames))
	}
	for i, gotFrame := range gotFrames {
		got := bridgeOutputData(t, gotFrame)
		if got["type"] != "response.completed" && got["type"] != "response.incomplete" {
			if gotFrame != wantFrames[i] {
				t.Errorf("non-terminal event %d changed:\ngot  %s\nwant %s", i, gotFrame, wantFrames[i])
			}
			continue
		}
		want := bridgeOutputData(t, wantFrames[i])
		delete(got["response"].(map[string]any), "output")
		delete(want["response"].(map[string]any), "output")
		if !reflect.DeepEqual(got, want) {
			t.Errorf("terminal fields other than response.output changed:\ngot  %v\nwant %v", got, want)
		}
	}
}

func TestResponsesBridgeTerminalOutput_IdentityAllocationAndSnapshotUpdates(t *testing.T) {
	for _, terminalType := range []string{"response.completed", "response.incomplete"} {
		t.Run(terminalType, func(t *testing.T) {
			frames := newFrameCollector()
			bridge := NewResponsesBridge("fixture-model", nil, frames.write)
			t.Cleanup(bridge.StopCompletionWatchdog)
			itemEvent := func(eventType string, item map[string]any) translator.ResponsesEvent {
				return translator.ResponsesEvent{Event: eventType, Data: map[string]any{"type": eventType, "output_index": 0, "item": item}}
			}
			first := map[string]any{"id": "first", "type": "message", "content": []any{"final answer"}}
			second := map[string]any{"id": "second", "type": "custom_tool_call", "input": "final program"}
			third := map[string]any{"id": "third", "type": "function_call", "arguments": "{}"}
			events := []translator.ResponsesEvent{
				itemEvent("response.output_item.added", map[string]any{"id": "first", "type": "message"}),
				itemEvent("response.output_item.added", map[string]any{"id": "second", "type": "custom_tool_call"}),
				itemEvent("response.output_item.done", map[string]any{"id": "second", "type": "custom_tool_call", "input": "stale program"}),
				itemEvent("response.output_item.done", first),
				itemEvent("response.output_item.done", second),
				itemEvent("response.output_item.done", third),
				{Event: terminalType, Data: map[string]any{"type": terminalType, "response": map[string]any{"output": []any{third}}}},
			}
			var original strings.Builder
			for _, ev := range events {
				original.WriteString(translator.FormatResponsesSSE(ev))
			}
			if err := bridge.writeEvents(events); err != nil {
				t.Fatalf("writeEvents: %v", err)
			}
			body := frames.text()
			parts := strings.Split(strings.TrimSpace(body), "\n\n")
			terminal := bridgeOutputData(t, parts[len(parts)-1])["response"].(map[string]any)
			if want := []any{first, second, third}; !reflect.DeepEqual(terminal["output"], want) {
				t.Errorf("identities must retain added order, replace stale snapshots, and allocate done-only items:\ngot  %v\nwant %v", terminal["output"], want)
			}
			assertBridgeOutputOnlyTerminalChanged(t, body, original.String())
		})
	}
}

func TestResponsesBridgeTerminalOutput_WriteErrorStopsEmission(t *testing.T) {
	for _, path := range []string{"feed", "close", "watchdog"} {
		t.Run(path, func(t *testing.T) {
			withFlushTimeout(t, 40*time.Millisecond)
			frames := newFrameCollector()
			writeErr := errors.New("fixture writer closed")
			failed := make(chan struct{}, 1)
			bridge := NewResponsesBridge("fixture-model", nil, func(frame []byte) error {
				if path == "feed" || strings.Contains(string(frame), `"type":"response.completed"`) {
					failed <- struct{}{}
					return writeErr
				}
				return frames.write(frame)
			})
			t.Cleanup(bridge.Close)
			for _, payload := range bridgeOutputChunks(`{"b":2}`) {
				bridge.Feed([]byte(payload))
			}
			if path == "watchdog" {
				bridge.Feed([]byte(`{"id":"c16","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`))
			} else {
				bridge.Close()
			}
			select {
			case <-failed:
			case <-time.After(time.Second):
				t.Fatal("expected writer failure was not reached")
			}
			if !errors.Is(bridge.Err(), writeErr) {
				t.Fatalf("bridge error = %v, want %v", bridge.Err(), writeErr)
			}
			before := frames.text()
			bridge.Feed([]byte("[DONE]"))
			bridge.Close()
			if frames.text() != before || len(failed) != 0 {
				t.Fatal("bridge wrote again after the first writer failure")
			}
		})
	}
}
