package executor

import (
	"errors"
	"math"
	"strings"
)

type devinDesktopUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CacheWriteTokens         int `json:"cache_write_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

type devinDesktopCallFragment struct{ id, name, arguments string }

type devinDesktopResponse struct {
	text     string
	thinking string
	stop     uint64
	usage    *devinDesktopUsage
	calls    []devinDesktopCallFragment
}

func devinDesktopDecodeResponse(payload []byte) (devinDesktopResponse, error) {
	var response devinDesktopResponse
	err := devinDesktopFields(payload, func(f devinDesktopField) error {
		switch f.number {
		case 3, 9:
			if err := f.require(2); err != nil {
				return err
			}
			if f.number == 3 {
				response.text += string(f.bytes)
			} else {
				response.thinking += string(f.bytes)
			}
		case 5:
			if err := f.require(0); err != nil {
				return err
			}
			response.stop = f.value
		case 6:
			if err := f.require(2); err != nil {
				return err
			}
			call, err := devinDesktopDecodeCall(f.bytes)
			if err != nil {
				return err
			}
			if call.id != "" || call.name != "" || call.arguments != "" {
				response.calls = append(response.calls, call)
			}
		case 7:
			if err := f.require(2); err != nil {
				return err
			}
			usage, err := devinDesktopDecodeUsage(f.bytes)
			if err != nil {
				return err
			}
			response.usage = usage
		}
		return nil
	})
	return response, err
}

func devinDesktopDecodeCall(payload []byte) (devinDesktopCallFragment, error) {
	var call devinDesktopCallFragment
	err := devinDesktopFields(payload, func(f devinDesktopField) error {
		if f.number < 1 || f.number > 3 {
			return nil
		}
		if err := f.require(2); err != nil {
			return err
		}
		switch f.number {
		case 1:
			call.id = string(f.bytes)
		case 2:
			call.name = string(f.bytes)
		case 3:
			call.arguments += string(f.bytes)
		}
		return nil
	})
	return call, err
}

func devinDesktopDecodeUsage(payload []byte) (*devinDesktopUsage, error) {
	usage := &devinDesktopUsage{}
	err := devinDesktopFields(payload, func(f devinDesktopField) error {
		if f.number < 2 || f.number > 5 {
			return nil
		}
		if err := f.require(0); err != nil {
			return err
		}
		if f.value > uint64(math.MaxInt)/2 {
			return errors.New("usage counter overflow")
		}
		switch f.number {
		case 2:
			usage.PromptTokens = int(f.value)
		case 3:
			usage.CompletionTokens = int(f.value)
		case 4:
			usage.CacheWriteTokens, usage.CacheCreationInputTokens = int(f.value), int(f.value)
		case 5:
			usage.PromptTokensDetails.CachedTokens = int(f.value)
		}
		return nil
	})
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage, err
}

func devinDesktopFinish(stop uint64, tools bool) string {
	if tools || stop == 10 {
		return "tool_calls"
	}
	if stop == 3 {
		return "length"
	}
	if stop == 11 {
		return "content_filter"
	}
	return "stop"
}

type devinDesktopDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type devinDesktopDeltaCall struct {
	Index    int                       `json:"index"`
	ID       string                    `json:"id,omitempty"`
	Type     string                    `json:"type,omitempty"`
	Function devinDesktopDeltaFunction `json:"function"`
}

type devinDesktopDelta struct {
	Role      string                  `json:"role,omitempty"`
	Content   string                  `json:"content,omitempty"`
	Thinking  string                  `json:"reasoning_content,omitempty"`
	ToolCalls []devinDesktopDeltaCall `json:"tool_calls,omitempty"`
}

type devinDesktopCollectedCall struct {
	ID       string                    `json:"id"`
	Type     string                    `json:"type"`
	Function devinDesktopDeltaFunction `json:"function"`
}

type devinDesktopCollected struct {
	text     strings.Builder
	thinking strings.Builder
	calls    []devinDesktopCollectedCall
	bytes    int
}
