// 9RouterX — native executor for the OpenCode CLI Server (OmniRoute 632d84f5).
// The local server is keyless: no credential is read, forwarded or stored here.
package executor

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

// ForwardOpenCodeServer serves the pinned v1 CLI Server protocol through one
// dedicated executor. It never falls through to ForwardOpencode: the free
// inference gateway and the local server have different endpoints, catalogs
// and session lifecycles.
func ForwardOpenCodeServer(w http.ResponseWriter, req *Request) error {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := opencodeServerClient(req)
	base, err := opencodeServerBaseURL(req)
	if err != nil {
		return err
	}
	if err := opencodeServerCheckHealth(ctx, client, base); err != nil {
		return err
	}
	input, ctx, err := opencodeServerInputFrom(ctx, req)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}
	model := req.ModelName
	if model == "" {
		model = input.model
	}
	resolved, err := opencodeServerResolveModel(ctx, client, base, model)
	if err != nil {
		return err
	}
	body := opencodeServerMessageBody{
		Model:  map[string]string{"providerID": resolved.ProviderID, "modelID": model},
		System: input.system,
		Tools:  map[string]any{},
		Parts:  input.parts,
	}
	sessionID, err := opencodeServerCreateSession(ctx, client, base, input.title)
	if err != nil {
		return err
	}
	defer opencodeServerCleanup(client, base, sessionID)
	if req.IsStream {
		return opencodeServerStream(ctx, w, req, client, base, sessionID, model, body)
	}
	result, err := opencodeServerBlockingMessage(ctx, client, base, sessionID, body)
	if err != nil {
		return err
	}
	return opencodeServerAnswer(ctx, w, req, result, model)
}

type opencodeServerStreamState struct {
	req           *Request
	writer        *proxy.HeartbeatWriter
	responses     *opencodeServerResponses
	claudeSession string
	id            string
	model         string
}

func opencodeServerStream(ctx context.Context, w http.ResponseWriter, req *Request, client *http.Client, base, sessionID, model string, body opencodeServerMessageBody) error {
	sub, err := opencodeServerSubscribe(ctx, client, base)
	if err != nil {
		return err
	}
	defer sub.close()
	state := &opencodeServerStreamState{
		req: req, writer: proxy.NewHeartbeatWriter(ctx, w, 0), id: "ocs-" + sessionID, model: model,
	}
	defer func() { _ = state.writer.Close() }()
	proxy.WriteSSEHeaders(state.writer)
	if translator.IsResponsesClient(ctx) {
		state.responses = &opencodeServerResponses{state: translator.InitResponsesState(model, true)}
	} else if req.TranslateResp {
		state.claudeSession = translator.NewSessionKey("opencode-server")
		defer translator.ClearStreamState(state.claudeSession)
	}
	if err := state.role(); err != nil {
		return err
	}
	if err := opencodeServerPromptAsync(ctx, client, base, sessionID, body); err != nil {
		return state.failure(err)
	}
	return opencodeServerPump(ctx, state, sub, sessionID)
}

type opencodeServerResponses struct {
	state *translator.ResponsesState
}

func (b *opencodeServerResponses) feed(chunk *translator.OpenAIChunk) []translator.ResponsesEvent {
	return translator.TranslateOpenAIToResponses(chunk, b.state)
}

func (b *opencodeServerResponses) flush() []translator.ResponsesEvent {
	return translator.FlushResponses(b.state)
}

func (s *opencodeServerStreamState) role() error {
	return s.chunk(translator.OpenAIDelta{Role: "assistant"}, nil, nil)
}

func (s *opencodeServerStreamState) chunk(delta translator.OpenAIDelta, usage *translator.OpenAIUsage, finish *string) error {
	chunk := translator.OpenAIChunk{
		ID: s.id, Model: s.model,
		Choices: []translator.OpenAIChoice{{Index: 0, Delta: delta, FinishReason: finish}},
		Usage:   usage,
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("encode OpenCode chunk: %w", err)
	}
	return s.emit(encoded)
}

func (s *opencodeServerStreamState) usage(tokens opencodeServerTokenCounts) error {
	u := tokens.usage()
	if u == nil {
		return nil
	}
	chunk := translator.OpenAIChunk{ID: s.id, Model: s.model, Choices: []translator.OpenAIChoice{}, Usage: u}
	return s.feed(&chunk)
}

func (s *opencodeServerStreamState) emit(payload []byte) error {
	if s.req.ResponseBuf != nil {
		if _, err := s.req.ResponseBuf.Write(payload); err != nil {
			return fmt.Errorf("capture OpenCode stream: %w", err)
		}
	}
	if s.req.TTFT != nil && *s.req.TTFT == 0 && !s.req.StartTime.IsZero() {
		*s.req.TTFT = time.Since(s.req.StartTime).Milliseconds()
	}
	return s.write(payload)
}

func (s *opencodeServerStreamState) write(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if _, err := s.writer.Write(payload); err != nil {
		return fmt.Errorf("write OpenCode stream: %w", err)
	}
	s.writer.Flush()
	return nil
}

func (s *opencodeServerStreamState) frame(data []byte) error {
	return s.write(append(append([]byte("data: "), data...), '\n', '\n'))
}

func (s *opencodeServerStreamState) feed(chunk *translator.OpenAIChunk) error {
	if s.responses != nil {
		for _, event := range s.responses.feed(chunk) {
			if err := s.write([]byte(translator.FormatResponsesSSE(event))); err != nil {
				return err
			}
		}
		return nil
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("encode OpenCode chunk: %w", err)
	}
	if s.claudeSession != "" {
		frame, err := translator.TranslateOpenAIToClaudeStreamSession(s.claudeSession, encoded)
		if err != nil {
			return fmt.Errorf("translate OpenCode Claude chunk: %w", err)
		}
		return s.write(frame)
	}
	if len(chunk.Choices) == 0 {
		return s.frame(encoded)
	}
	return s.frame(encoded)
}

func (s *opencodeServerStreamState) failure(err error) error {
	message := "OpenCode CLI Server stream failed"
	if err != nil {
		message += ": " + err.Error()
	}
	if s.responses != nil {
		return s.write([]byte(translator.FormatResponsesFailedSSE(map[string]any{"code": "upstream_error", "message": message})))
	}
	format := proxy.SSEFormatOpenAI
	if s.claudeSession != "" {
		format = proxy.SSEFormatClaude
	}
	return s.write(proxy.BuildStreamErrorBytes(http.StatusBadGateway, message, format))
}
