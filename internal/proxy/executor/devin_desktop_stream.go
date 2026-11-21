package executor

import (
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

type devinDesktopStreamWriter struct {
	req             *Request
	writer          *proxy.HeartbeatWriter
	responses       *devinDesktopResponsesBridge
	claudeSession   string
	translatedBytes int
}

func devinDesktopStream(w http.ResponseWriter, req *Request, upstream io.Reader, encoding string, output *devinDesktopOutput) error {
	hw := proxy.NewHeartbeatWriter(req.Ctx, w, 0)
	defer hw.Close()
	proxy.WriteSSEHeaders(hw)
	sink := devinDesktopStreamWriter{req: req, writer: hw}
	if translator.IsResponsesClient(req.Ctx) {
		sink.responses = newDevinDesktopResponsesBridge(output.model, translator.CustomToolNamesFrom(req.Ctx))
	} else if req.TranslateResp {
		sink.claudeSession = translator.NewSessionKey("devin-desktop")
		defer translator.ClearStreamState(sink.claudeSession)
	}
	err := devinDesktopFrames(req.Ctx, upstream, encoding, func(response devinDesktopResponse) error {
		return output.consume(response, func(delta devinDesktopDelta) error {
			payload, err := output.chunk(delta, false)
			if err != nil {
				return err
			}
			return sink.feed(payload)
		})
	})
	if err != nil {
		if req.Ctx.Err() != nil {
			return fmt.Errorf("devin desktop stream: %w", req.Ctx.Err())
		}
		if writeErr := sink.failure(); writeErr != nil {
			return fmt.Errorf("devin desktop error frame: %w", writeErr)
		}
		return devinDesktopFailure(req.Ctx, "stream", err)
	}
	payload, err := output.chunk(devinDesktopDelta{}, true)
	if err != nil {
		return fmt.Errorf("devin desktop terminal: %w", err)
	}
	if err := sink.feed(payload); err != nil {
		return fmt.Errorf("devin desktop terminal: %w", err)
	}
	if err := sink.feed([]byte("[DONE]")); err != nil {
		return fmt.Errorf("devin desktop finish: %w", err)
	}
	if output.usage != nil {
		u := output.usage
		translator.SetUsage(req.Ctx, &translator.OpenAIUsage{
			PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
			CachedTokens: u.PromptTokensDetails.CachedTokens, CacheCreationInputTokens: u.CacheWriteTokens,
			PromptCacheIncluded: true,
		})
	}
	return nil
}

func (s *devinDesktopStreamWriter) write(frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	if s.req.TTFT != nil && *s.req.TTFT == 0 && !s.req.StartTime.IsZero() {
		*s.req.TTFT = time.Since(s.req.StartTime).Milliseconds()
	}
	if s.req.ResponseBuf != nil {
		if _, err := s.req.ResponseBuf.Write(frame); err != nil {
			return fmt.Errorf("capture response: %w", err)
		}
	}
	if _, err := s.writer.Write(frame); err != nil {
		return fmt.Errorf("write client: %w", err)
	}
	s.writer.Flush()
	return nil
}

func (s *devinDesktopStreamWriter) feed(payload []byte) error {
	if s.responses != nil || s.claudeSession != "" {
		s.translatedBytes += len(payload)
		if s.translatedBytes > devinDesktopFrameLimit {
			return fmt.Errorf("translated completion exceeds safety limit")
		}
	}
	if s.responses != nil {
		var chunk *translator.OpenAIChunk
		if string(payload) != "[DONE]" {
			chunk = new(translator.OpenAIChunk)
			if err := json.Unmarshal(payload, chunk); err != nil {
				return fmt.Errorf("decode bridge chunk: %w", err)
			}
		}
		for _, event := range s.responses.feed(chunk) {
			if err := s.write([]byte(translator.FormatResponsesSSE(event))); err != nil {
				return err
			}
		}
		return nil
	}
	if s.claudeSession != "" {
		frame, err := translator.TranslateOpenAIToClaudeStreamSession(s.claudeSession, payload)
		if err != nil {
			return fmt.Errorf("translate Claude chunk: %w", err)
		}
		return s.write(frame)
	}
	return s.write(append(append([]byte("data: "), payload...), '\n', '\n'))
}

func (s *devinDesktopStreamWriter) failure() error {
	const message = "Devin Desktop stream failed"
	if s.responses != nil {
		return s.write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"" + s.responses.state.ResponseID + "\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"upstream_error\",\"message\":\"" + message + "\"}}}\n\n"))
	}
	format := proxy.SSEFormatOpenAI
	if s.claudeSession != "" {
		format = proxy.SSEFormatClaude
	}
	return s.write(proxy.BuildStreamErrorBytes(http.StatusBadGateway, message, format))
}
