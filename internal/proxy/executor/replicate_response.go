// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
package executor

import (
	"math"
	"strings"
	"time"

	json "encoding/json/v2"

	"github.com/google/uuid"
)

type replicateUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func replicateEstimateTokens(text string) int {
	if n := (len(text) + 3) / 4; n > 1 {
		return n
	}
	return 1
}

// replicateMetric accepts a reported zero; only a missing or invalid metric
// falls back to the estimate.
func replicateMetric(prediction map[string]any, key string) (int, bool) {
	value, ok := replicateMap(prediction["metrics"])[key].(float64)
	if !ok || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	return int(value), true
}

func replicateResolveUsage(prediction map[string]any, promptText, content string) replicateUsage {
	promptTokens, ok := replicateMetric(prediction, "input_token_count")
	if !ok {
		promptTokens = replicateEstimateTokens(promptText)
	}
	completionTokens, ok := replicateMetric(prediction, "output_token_count")
	if !ok {
		completionTokens = replicateEstimateTokens(content)
	}
	return replicateUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
}

func replicateFinishReason(usage replicateUsage, maxTokens *float64) string {
	if maxTokens != nil && float64(usage.CompletionTokens) >= *maxTokens {
		return "length"
	}
	return "stop"
}

func replicateOutputToText(output any) string {
	if s, ok := output.(string); ok {
		return s
	}
	parts, ok := output.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if s, ok := part.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}

type replicateChunks struct {
	id      string
	created int64
	model   string
}

type replicateDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type replicateChoice struct {
	Index        int            `json:"index"`
	Delta        replicateDelta `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type replicateChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []replicateChoice `json:"choices"`
	Usage   *replicateUsage   `json:"usage,omitempty"`
}

func newReplicateChunks(model string) replicateChunks {
	return replicateChunks{id: "chatcmpl-replicate-" + uuid.NewString(), created: time.Now().Unix(), model: model}
}

func (c replicateChunks) frame(choices []replicateChoice, usage *replicateUsage) []byte {
	if choices == nil {
		choices = []replicateChoice{}
	}
	chunk := replicateChunk{ID: c.id, Object: "chat.completion.chunk", Created: c.created, Model: c.model, Choices: choices, Usage: usage}
	body, _ := json.Marshal(chunk, json.Deterministic(true))
	return append([]byte("data: "), append(body, '\n', '\n')...)
}

func (c replicateChunks) roleFrame() []byte {
	return c.frame([]replicateChoice{{Index: 0, Delta: replicateDelta{Role: "assistant"}}}, nil)
}

func (c replicateChunks) contentFrame(text string) []byte {
	return c.frame([]replicateChoice{{Index: 0, Delta: replicateDelta{Content: text}}}, nil)
}

func (c replicateChunks) finishFrame(reason string) []byte {
	return c.frame([]replicateChoice{{Index: 0, FinishReason: &reason}}, nil)
}

func (c replicateChunks) usageFrame(usage replicateUsage) []byte {
	return c.frame(nil, &usage)
}
