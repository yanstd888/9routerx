package executor

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestDevinDesktopNativeModelDiscovery(t *testing.T) {
	// Config fields 1 name, 4 disabled, 5 vision, 18 context, 22 literal ID.
	model := append(devinDesktopTestString(1, "Small"), devinDesktopTestString(22, "gpt-5-6-sol-low")...)
	model = append(model, devinDesktopTestNumber(18, 32000)...)
	model = append(model, devinDesktopTestNumber(5, 1)...)
	disabled := append(devinDesktopTestString(22, "disabled"), devinDesktopTestNumber(4, 1)...)
	fallback := devinDesktopTestString(22, "claude-opus-5-max")
	replacement := append(devinDesktopTestString(1, "Updated"), devinDesktopTestString(22, "gpt-5-6-sol-low")...)
	payload := append(devinDesktopTestString(1, string(model)), devinDesktopTestString(1, string(disabled))...)
	payload = append(payload, devinDesktopTestString(1, string(fallback))...)
	payload = append(payload, devinDesktopTestString(1, string(replacement))...)
	for _, compressed := range []bool{false, true} {
		name := "plain"
		if compressed {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			wire := payload
			if compressed {
				wire = devinDesktopTestGzip(t, payload)
			}
			resp, body := devinDesktopTestReply(wire)
			calls := 0
			client := &http.Client{Transport: devinDesktopTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Path != devinDesktopModelsPath || req.Header.Get("Content-Type") != "application/proto" || req.Header.Get("Connect-Protocol-Version") != "1" || req.Header.Get("Authorization") != "" {
					t.Error("not native discovery")
				}
				deadline, ok := req.Context().Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Error("discovery timeout missing")
				}
				request, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				fields := devinDesktopTestFields(t, request)
				metadata := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 1).bytes)
				if string(devinDesktopTestField(t, metadata, 3).bytes) != "fixture-key" || len(metadata[21]) != 0 {
					t.Error("discovery authentication")
				}
				return resp, nil
			})}
			models, err := FetchDevinDesktopModels(context.Background(), client, "fixture-key", "https://server.codeium.com"+devinDesktopChatPath)
			if err != nil {
				t.Fatal(err)
			}
			want := []DevinDesktopModel{{ID: "gpt-5-6-sol-low", Name: "Updated", ContextLength: 200000, MaxOutputTokens: 64000}, {ID: "claude-opus-5-max", Name: "claude-opus-5-max", ContextLength: 200000, MaxOutputTokens: 64000}}
			if !reflect.DeepEqual(models, want) || calls != 1 || !body.closed.Load() {
				t.Fatalf("models=%+v calls=%d closed=%v", models, calls, body.closed.Load())
			}
		})
	}
	models, err := devinDesktopDecodeModels(devinDesktopTestString(1, string(model)))
	if err != nil || len(models) != 1 || !models[0].SupportsVision || models[0].ContextLength != 32000 || models[0].MaxOutputTokens != 32000 {
		t.Errorf("small vision model=%+v err=%v", models, err)
	}
}

func TestDevinDesktopModelDiscoveryFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payload  []byte
		declared int64
		status   int
	}{
		{"bad-config", []byte{0x0a, 1, 0}, 0, 200},
		{"invalid-gzip", []byte{0x1f, 0x8b, 0}, 0, 200},
		{"oversized-header", nil, devinDesktopFrameLimit + 1, 200},
		{"upstream", []byte("fixture-key"), 0, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := devinDesktopTestReply(tc.payload)
			resp.StatusCode = tc.status
			if tc.declared != 0 {
				resp.ContentLength = tc.declared
			}
			client := &http.Client{Transport: devinDesktopTestTransport(func(*http.Request) (*http.Response, error) { return resp, nil })}
			_, err := FetchDevinDesktopModels(context.Background(), client, "fixture-key", "")
			if err == nil || !body.closed.Load() {
				t.Fatalf("err=%v closed=%v", err, body.closed.Load())
			}
		})
	}
}
