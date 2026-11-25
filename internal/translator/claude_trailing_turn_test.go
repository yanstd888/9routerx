// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package translator

import (
	json "encoding/json/v2"
	"testing"
)

// Newer Claude models answer 400 "… does not support assistant message
// prefill" when the conversation ends on an assistant turn. The cleanup in
// SanitizeClaudePassthrough deletes messages left empty, so a trailing user
// turn that was blank (or held only dropped blocks) silently promotes the
// previous assistant turn to last. Upstream restores a placeholder user turn
// for that case and leaves a prefill the client wrote itself alone
// (open-sse/translator/formats/claude.js:345-349).

func claudeTurn(role string, blocks ...any) map[string]any {
	return map[string]any{"role": role, "content": blocks}
}

func textBlock(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

func messageRoles(t *testing.T, body []byte) []string {
	t.Helper()
	var decoded struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal sanitized body: %v (%s)", err, body)
	}
	roles := make([]string, 0, len(decoded.Messages))
	for _, m := range decoded.Messages {
		roles = append(roles, m.Role)
	}
	return roles
}

// TestSanitizeClaudePassthrough_CleanupInducedAssistantTail: the empty trailing
// user turn is dropped, so the repair must put a user turn back — otherwise the
// upstream 400s on a body the client never wrote that way.
func TestSanitizeClaudePassthrough_CleanupInducedAssistantTail(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5","messages":[
		{"role":"user","content":[{"type":"text","text":"hi"}]},
		{"role":"assistant","content":[{"type":"text","text":"working on it"}]},
		{"role":"user","content":[{"type":"text","text":"   "}]}
	]}`)

	got := SanitizeClaudePassthrough(body, false)

	roles := messageRoles(t, got)
	want := []string{"user", "assistant", "user"}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v, want %v (body: %s)", roles, want, got)
	}
	for i, role := range want {
		if roles[i] != role {
			t.Fatalf("roles = %v, want %v (body: %s)", roles, want, got)
		}
	}
	if !containsPlaceholder(got) {
		t.Errorf("body must carry the trailing user placeholder, got %s", got)
	}
}

// TestSanitizeClaudePassthrough_ClientPrefillPreserved is the other direction: a
// body the client deliberately ended on an assistant turn keeps it, because the
// prefill is the client's own choice and only older models accept it.
func TestSanitizeClaudePassthrough_ClientPrefillPreserved(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5","messages":[
		{"role":"user","content":[{"type":"text","text":"Write a haiku"}]},
		{"role":"assistant","content":[{"type":"text","text":"Once upon a"}]}
	]}`)

	if !ClaudeIntentionalPrefill(body) {
		t.Fatal("a trailing assistant turn in messages[] is a prefill")
	}
	got := SanitizeClaudePassthrough(body, ClaudeIntentionalPrefill(body))

	if containsPlaceholder(got) {
		t.Errorf("a client prefill must not get a placeholder appended: %s", got)
	}
	roles := messageRoles(t, got)
	if len(roles) != 2 || roles[1] != "assistant" {
		t.Fatalf("roles = %v, want the client's assistant tail kept", roles)
	}
}

// TestClaudeIntentionalPrefill reads the tail in the source format's own shape,
// because messages[] is not the only one a request arrives in.
func TestClaudeIntentionalPrefill(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "claude client tail",
			body: `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"prefill"}]}`,
			want: true,
		},
		{
			name: "claude client ending on a user turn",
			body: `{"messages":[{"role":"assistant","content":"x"},{"role":"user","content":"hi"}]}`,
			want: false,
		},
		{
			name: "gemini/Antigravity model tail",
			body: `{"contents":[{"role":"user","parts":[]},{"role":"model","parts":[]}]}`,
			want: true,
		},
		{
			name: "responses/codex assistant tail",
			body: `{"input":[{"role":"user","content":"hi"},{"role":"assistant","content":"prefill"}]}`,
			want: true,
		},
		{
			name: "function output tail carries no role",
			body: `{"input":[{"role":"assistant","content":"x"},{"type":"function_call_output","call_id":"c1"}]}`,
			want: false,
		},
		{
			name: "no conversation at all",
			body: `{"model":"claude-opus-5"}`,
			want: false,
		},
		{
			name: "not JSON",
			body: `{`,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClaudeIntentionalPrefill([]byte(tt.body)); got != tt.want {
				t.Errorf("ClaudeIntentionalPrefill(%s) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestEnsureTrailingUserTurnBody covers the path that runs after the OpenAI→
// Claude conversion, where the merged turns can end on an assistant message even
// though the client body ended on a tool result.
func TestEnsureTrailingUserTurnBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		prefill    bool
		wantRepair bool
	}{
		{
			name:       "assistant tail gets the placeholder",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"text","text":"partial"}]}]}`,
			prefill:    false,
			wantRepair: true,
		},
		{
			name:       "client prefill is left alone",
			body:       `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"text","text":"partial"}]}]}`,
			prefill:    true,
			wantRepair: false,
		},
		{
			name:       "user tail needs no repair",
			body:       `{"messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}]},{"role":"user","content":[{"type":"text","text":"go on"}]}]}`,
			prefill:    false,
			wantRepair: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureTrailingUserTurnBody([]byte(tt.body), tt.prefill)
			repaired := containsPlaceholder(got)
			if repaired != tt.wantRepair {
				t.Fatalf("placeholder appended = %v, want %v (body: %s)", repaired, tt.wantRepair, got)
			}
			if !tt.wantRepair && string(got) != tt.body {
				t.Errorf("body must be returned unchanged, got %s", got)
			}
		})
	}
}

func TestEnsureTrailingUserTurnBodyReplacesEmptiedTail(t *testing.T) {
	for _, tt := range []struct {
		name    string
		emptied string
	}{
		{"empty block array", `[]`},
		{"blank string", `"   "`},
		{"null content", `null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{"model":"claude-sonnet","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"assistant","content":[{"type":"text","text":"partial"}]},{"role":"user","content":` + tt.emptied + `}]}`)

			got := EnsureTrailingUserTurnBody(body, false)
			if string(got) == string(body) {
				t.Fatal("body wrapper returned the original body; emptied tail replacement was lost")
			}
			if !containsPlaceholder(got) {
				t.Fatalf("placeholder missing from repaired body: %s", got)
			}
			var decoded struct {
				Messages []struct {
					Role    string `json:"role"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("decode repaired body: %v", err)
			}
			if len(decoded.Messages) != 3 || decoded.Messages[0].Role != "user" || decoded.Messages[1].Role != "assistant" || decoded.Messages[2].Role != "user" || len(decoded.Messages[2].Content) != 1 || decoded.Messages[2].Content[0].Text != TrailingUserPlaceholder {
				t.Fatalf("body wrapper did not replace only the emptied tail: %+v", decoded.Messages)
			}
		})
	}
}

func containsPlaceholder(body []byte) bool {
	var decoded struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false
	}
	for _, m := range decoded.Messages {
		for _, b := range m.Content {
			if b.Type == "text" && b.Text == TrailingUserPlaceholder {
				return true
			}
		}
	}
	return false
}

// TestEnsureTrailingUserTurn_ReplacesEmptiedTail: a trailing user turn whose
// content the cleanup emptied is REPLACED by the placeholder. The emptied turn
// must not ride along next to it — Anthropic rejects empty content just as
// hard as a missing tail, so shipping both turns answers 400 for a body the
// client never wrote that way.
func TestEnsureTrailingUserTurn_ReplacesEmptiedTail(t *testing.T) {
	for _, tt := range []struct {
		name    string
		emptied map[string]any
	}{
		{"empty block array", map[string]any{"role": "user", "content": []any{}}},
		{"blank string", map[string]any{"role": "user", "content": "   "}},
		{"null content", map[string]any{"role": "user", "content": nil}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			messages := []any{
				claudeTurn("user", textBlock("hi")),
				claudeTurn("assistant", textBlock("partial")),
				tt.emptied,
			}

			repaired := EnsureTrailingUserTurn(messages, false)

			if len(repaired) != len(messages) {
				t.Fatalf("repaired length = %d, want %d (emptied tail replaced, not kept): %v",
					len(repaired), len(messages), repaired)
			}
			for i, want := range []string{"user", "assistant", "user"} {
				role, _ := repaired[i].(map[string]any)["role"].(string)
				if role != want {
					t.Fatalf("role[%d] = %q, want %q (messages: %v)", i, role, want, repaired)
				}
			}
			if !hasContent(repaired[len(repaired)-1].(map[string]any)["content"]) {
				t.Errorf("final turn has no usable content: %v", repaired[len(repaired)-1])
			}
			if !containsPlaceholder(encodeMessages(t, repaired)) {
				t.Errorf("placeholder missing from repaired tail: %v", repaired)
			}
			// The caller's own slice must be untouched by the repair.
			if _, ok := messages[2].(map[string]any)["role"]; !ok {
				t.Errorf("caller's slice was mutated: %v", messages)
			}
		})
	}
}

// encodeMessages marshals a repaired message list the way
// EnsureTrailingUserTurnBody would, so containsPlaceholder can read it.
func encodeMessages(t *testing.T, messages []any) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{"messages": messages}, json.Deterministic(true))
	if err != nil {
		t.Fatalf("marshal repaired messages: %v", err)
	}
	return out
}

// sameTail decides whether cleanup changed the tail of a conversation, and the
// caller re-encodes the whole body when it says yes. It compared the two
// messages by their JSON text, which cannot work: json/v2 marshals a Go map in
// a non-deterministic order (v1 sorted the keys, v2 does not), so two identical
// messages serialise differently about one time in ten. A clean body was then
// re-encoded for no reason — and, because that re-encoding is itself
// order-random, the same request produced different bytes on every attempt.
func TestSameTail_ComparesByValueNotByJSONText(t *testing.T) {
	tail := func() []any {
		return []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "user", "content": "again"},
		}
	}

	// Enough attempts that a text comparison would fail here reliably: the
	// observed rate was roughly one in ten.
	for i := 0; i < 500; i++ {
		if !sameTail(tail(), tail()) {
			t.Fatalf("identical tails reported as different on attempt %d", i)
		}
	}

	changed := tail()
	changed[len(changed)-1] = map[string]any{"role": "user", "content": "different"}
	if sameTail(changed, tail()) {
		t.Fatal("a genuinely different tail was reported as unchanged")
	}

	// A tail that gained the placeholder is a change even though the shared
	// prefix is identical.
	longer := append(tail(), map[string]any{
		"role":    "user",
		"content": []any{map[string]any{"type": "text", "text": TrailingUserPlaceholder}},
	})
	if sameTail(longer, tail()) {
		t.Fatal("a repaired tail was reported as unchanged")
	}
}
