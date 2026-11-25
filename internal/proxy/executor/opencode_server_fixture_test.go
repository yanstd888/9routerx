package executor

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"9routerx/proxy/internal/providers"
)

type opencodeServerRecord struct {
	method string
	path   string
	header http.Header
	body   []byte
}

type opencodeServerFixture struct {
	t          *testing.T
	server     *httptest.Server
	mu         sync.Mutex
	records    []opencodeServerRecord
	events     []string
	release    chan struct{}
	released   bool
	version    string
	healthy    bool
	probe      int
	catalog    []byte
	message    []byte
	sessionID  string
	promptCode int
	onPrompt   func()
}

const opencodeServerFixtureCatalog = `{"providers":[{"id":"opencode","models":{"big-pickle":{"cost":{"input":0,"output":0}},"space-bunny-free":{"cost":{"input":0,"output":0}}}},{"id":"opencode-zen","models":{"big-pickle":{"cost":{"input":1,"output":2}},"muse-spark-1.3-contributor-free":{"cost":{"input":0,"output":0}}}}]}`

func newOpenCodeServerFixture(t *testing.T) *opencodeServerFixture {
	t.Helper()
	f := &opencodeServerFixture{
		t: t, version: "1.2.3", healthy: true, probe: http.StatusNotFound,
		sessionID: "ses_fixture", promptCode: http.StatusNoContent,
		catalog: []byte(opencodeServerFixtureCatalog),
		message: []byte(`{"info":{"id":"msg_1","role":"assistant","sessionID":"ses_fixture","tokens":{"input":12,"output":7,"reasoning":3,"cache":{"read":4,"write":2}}},"parts":[{"type":"text","text":"Hello"},{"type":"reasoning","text":"Thinking"}]}`),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/global/health", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, nil)
		fmt.Fprintf(w, `{"healthy":%t,"version":%q}`, f.healthy, f.version)
	})
	mux.HandleFunc("/api/provider", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, nil)
		w.WriteHeader(f.probe)
	})
	mux.HandleFunc("/config/providers", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, nil)
		w.Write(f.catalog)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, nil)
		fmt.Fprintf(w, `{"id":%q}`, f.sessionID)
	})
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		body := f.record(r, nil)
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt_async"):
			w.WriteHeader(f.promptCode)
			f.mu.Lock()
			callback, released := f.onPrompt, f.released
			if !released {
				f.released = true
				close(f.release)
			}
			f.mu.Unlock()
			if callback != nil {
				callback()
			}
		case r.Method == http.MethodPost:
			_ = body
			w.Write(f.message)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/event", func(w http.ResponseWriter, r *http.Request) {
		f.record(r, nil)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: server.connected\ndata: {\"type\":\"server.connected\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		select {
		case <-f.release:
		case <-r.Context().Done():
			return
		}
		f.mu.Lock()
		frames := append([]string(nil), f.events...)
		f.mu.Unlock()
		for _, frame := range frames {
			fmt.Fprint(w, frame)
			if flusher != nil {
				flusher.Flush()
			}
		}
		<-r.Context().Done()
	})
	f.server = httptest.NewServer(mux)
	f.release = make(chan struct{})
	t.Cleanup(func() {
		f.server.Close()
		opencodeServerCatalogMu.Lock()
		delete(opencodeServerCatalogs, f.server.URL)
		opencodeServerCatalogMu.Unlock()
		opencodeServerBusMu.Lock()
		if bus := opencodeServerBuses[f.server.URL]; bus != nil {
			bus.stop()
		}
		opencodeServerBusMu.Unlock()
	})
	return f
}

func (f *opencodeServerFixture) record(r *http.Request, _ any) []byte {
	body := make([]byte, 0)
	if r.Body != nil {
		body = opencodeServerReadBody(r)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, opencodeServerRecord{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body})
	return body
}

func (f *opencodeServerFixture) calls(path string) []opencodeServerRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []opencodeServerRecord
	for _, record := range f.records {
		if record.path == path {
			out = append(out, record)
		}
	}
	return out
}

func (f *opencodeServerFixture) setEvents(frames ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = frames
}

func opencodeServerReadBody(r *http.Request) []byte {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return body
}

func opencodeServerRequest(f *opencodeServerFixture, model string, stream bool, overrides map[string]any) *Request {
	payload := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{"role": "system", "content": "Be brief"},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Hello"}}},
		},
		"stream": stream,
	}
	for key, value := range overrides {
		payload[key] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		f.t.Fatalf("marshal fixture request: %v", err)
	}
	req := &Request{
		Ctx: context.Background(), Client: f.server.Client(),
		Config:   &providers.ProviderConfig{BaseURL: f.server.URL},
		Provider: "opencode-server", ModelName: model, Body: body, IsStream: stream,
	}
	return req
}
