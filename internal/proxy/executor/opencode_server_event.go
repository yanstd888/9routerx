package executor

import (
	"bufio"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"9routerx/proxy/internal/proxy"
)

type opencodeServerEvent struct {
	Type      string
	SessionID string
	MessageID string
	PartID    string
	Role      string
	Kind      string // text, reasoning or tool
	Delta     string
	Text      string
	Reasoning string
	Tokens    opencodeServerTokenCounts
	HasTokens bool
	Error     string
}

type opencodeServerBus struct {
	base   string
	client *http.Client
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	once   sync.Once

	mu        sync.Mutex
	subs      map[chan opencodeServerEvent]struct{}
	err       error
	readyOnce sync.Once
}

var (
	opencodeServerBusMu sync.Mutex
	opencodeServerBuses = map[string]*opencodeServerBus{}
)

type opencodeServerSubscription struct {
	bus *opencodeServerBus
	ch  chan opencodeServerEvent
}

func opencodeServerSubscribe(ctx context.Context, client *http.Client, base string) (*opencodeServerSubscription, error) {
	opencodeServerBusMu.Lock()
	bus := opencodeServerBuses[base]
	if bus == nil {
		busCtx, cancel := context.WithCancel(context.Background())
		bus = &opencodeServerBus{
			base: base, client: client, ctx: busCtx, cancel: cancel,
			ready: make(chan struct{}), done: make(chan struct{}),
			subs: make(map[chan opencodeServerEvent]struct{}),
		}
		opencodeServerBuses[base] = bus
		go bus.run()
	}
	opencodeServerBusMu.Unlock()

	select {
	case <-bus.ready:
	case <-bus.done:
		return nil, proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode event stream unavailable")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ch := make(chan opencodeServerEvent, 256)
	bus.mu.Lock()
	defer bus.mu.Unlock()
	select {
	case <-bus.done:
		return nil, proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode event stream unavailable")
	default:
	}
	bus.subs[ch] = struct{}{}
	return &opencodeServerSubscription{bus: bus, ch: ch}, nil
}

func (s *opencodeServerSubscription) close() {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.mu.Lock()
	delete(s.bus.subs, s.ch)
	empty := len(s.bus.subs) == 0
	s.bus.mu.Unlock()
	if empty {
		s.bus.stop()
	}
}

func (b *opencodeServerBus) stop() {
	b.cancel()
	opencodeServerBusMu.Lock()
	if opencodeServerBuses[b.base] == b {
		delete(opencodeServerBuses, b.base)
	}
	opencodeServerBusMu.Unlock()
}

func (b *opencodeServerBus) run() {
	defer b.finish(nil)
	req, err := http.NewRequestWithContext(b.ctx, http.MethodGet, b.base+"/event", nil)
	if err != nil {
		b.finish(err)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := b.client.Do(req)
	if err != nil {
		b.finish(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b.finish(fmt.Errorf("event stream status %d", resp.StatusCode))
		return
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1<<30))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	name, data := "", ""
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		switch {
		case line == "":
			if data != "" {
				event := opencodeServerParseEvent(name, data)
				if event.Type == "server.connected" {
					b.readyOnce.Do(func() { close(b.ready) })
				} else {
					b.fanout(event)
				}
			}
			name, data = "", ""
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil && b.ctx.Err() == nil {
		b.finish(err)
	}
}

func (b *opencodeServerBus) finish(err error) {
	b.mu.Lock()
	if err != nil {
		b.err = err
	}
	b.once.Do(func() { close(b.done) })
	b.mu.Unlock()
	b.stop()
}

func (b *opencodeServerBus) fanout(event opencodeServerEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- event
		}
	}
}
func opencodeServerParseEvent(name string, data string) opencodeServerEvent {
	var root map[string]any
	_ = json.Unmarshal([]byte(data), &root)
	event := opencodeServerEvent{Type: name}
	if value := opencodeServerField(root, "type"); value != "" {
		event.Type = value
	}
	props, _ := root["properties"].(map[string]any)
	if props == nil {
		props = root
	}
	info, _ := props["info"].(map[string]any)
	part, _ := props["part"].(map[string]any)
	event.SessionID = opencodeServerField(props, "sessionID")
	if event.SessionID == "" {
		event.SessionID = opencodeServerField(info, "sessionID")
	}
	event.MessageID = opencodeServerField(props, "messageID")
	if event.MessageID == "" {
		event.MessageID = opencodeServerField(info, "id")
	}
	event.PartID = opencodeServerField(props, "partID")
	if event.PartID == "" {
		event.PartID = opencodeServerField(part, "id")
	}
	event.Role = opencodeServerField(info, "role")
	if tokens, ok := info["tokens"].(map[string]any); ok {
		event.Tokens = opencodeServerTokensFrom(tokens)
		event.HasTokens = true
	}
	switch event.Type {
	case "session.error":
		event.Error = opencodeServerError(props, root)
	case "message.part.delta":
		event.Kind = opencodeServerField(props, "field")
		if event.Kind == "" {
			event.Kind = opencodeServerField(part, "type")
		}
		event.Delta = opencodeServerField(props, "delta")
		if event.Delta == "" {
			event.Delta = opencodeServerField(props, "text")
		}
	case "message.part.updated":
		event.Kind = opencodeServerField(part, "type")
		event.Text = opencodeServerPartText(part)
		if event.Kind == "reasoning" {
			event.Reasoning = event.Text
			event.Text = ""
		}
		if event.Kind == "tool" {
			event.Text = opencodeServerToolText(part)
		}
	}
	if event.Kind == "" {
		event.Kind = "text"
	}
	return event
}
func opencodeServerField(entry map[string]any, key string) string {
	value, _ := entry[key].(string)
	return value
}
func opencodeServerError(maps ...map[string]any) string {
	for _, entry := range maps {
		switch value := entry["error"].(type) {
		case string:
			if value != "" {
				return value
			}
		case map[string]any:
			for _, key := range []string{"message", "data", "name"} {
				if text := opencodeServerField(value, key); text != "" {
					return text
				}
			}
		}
	}
	return "OpenCode CLI Server session failed"
}
func opencodeServerPartText(part map[string]any) string {
	if text := opencodeServerField(part, "text"); text != "" {
		return text
	}
	if text := opencodeServerField(part, "reasoning"); text != "" {
		return text
	}
	state, _ := part["state"].(map[string]any)
	return opencodeServerField(state, "text")
}
func opencodeServerToolText(part map[string]any) string {
	name := opencodeServerField(part, "tool")
	if name == "" {
		name = opencodeServerField(part, "name")
	}
	callID := opencodeServerField(part, "callID")
	if callID == "" {
		callID = opencodeServerField(part, "id")
	}
	arguments := "{}"
	if input, ok := part["input"]; ok {
		if encoded, err := json.Marshal(input); err == nil {
			arguments = string(encoded)
		}
	}
	return opencodeServerEncodeCall(callID, name, arguments, false)
}
func opencodeServerTokensFrom(tokens map[string]any) opencodeServerTokenCounts {
	encoded, err := json.Marshal(tokens)
	if err != nil {
		return opencodeServerTokenCounts{}
	}
	var result opencodeServerTokenCounts
	_ = json.Unmarshal(encoded, &result)
	return result
}
