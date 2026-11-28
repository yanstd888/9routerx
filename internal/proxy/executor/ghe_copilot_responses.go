package executor

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"

	"9routerx/proxy/internal/constants"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

func gheResponsesResponse(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	if !translator.IsResponsesClient(req.Ctx) {
		return handleCodexStream(w, req, upstream)
	}
	if req.IsStream {
		return gheNativeResponsesStream(w, req, upstream)
	}
	var completed []byte
	var eventErr error
	err := proxy.ScanStreamEvents(io.LimitReader(upstream, constants.MaxUpstreamBodyBytes+1), func(ev proxy.SSEEvent) bool {
		var event struct {
			Type     string         `json:"type"`
			Response jsontext.Value `json:"response"`
		}
		if json.Unmarshal(ev.Data, &event) != nil {
			return true
		}
		switch event.Type {
		case "response.completed":
			completed = append([]byte(nil), event.Response...)
		case "response.failed", "error":
			eventErr = codexUpstreamError(ev.Data)
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	if eventErr != nil {
		return eventErr
	}
	if len(completed) == 0 {
		return proxy.UpstreamFailure(http.StatusBadGateway, proxy.NoCompletionInStream)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completed), false, req.ResponseBuf)
}

func gheNativeResponsesStream(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	proxy.WriteSSEHeaders(w)
	flusher, _ := w.(http.Flusher)
	var writeErr, errorEvent error
	terminal := false
	err := proxy.ScanStreamEvents(upstream, func(ev proxy.SSEEvent) bool {
		if err := req.Ctx.Err(); err != nil {
			writeErr = err
			return false
		}
		if bytes.Equal(bytes.TrimSpace(ev.Data), []byte("[DONE]")) {
			return true
		}
		var event struct {
			Type     string         `json:"type"`
			Response jsontext.Value `json:"response"`
		}
		if json.Unmarshal(ev.Data, &event) == nil {
			if event.Type == "response.failed" || event.Type == "error" {
				errorEvent = codexUpstreamError(ev.Data)
			}
			if event.Type == "response.completed" {
				terminal = true
				translator.SetUsage(req.Ctx, translator.ParseResponsesUsage(event.Response))
			}
		}
		frame := []byte{}
		if ev.Event != "" {
			frame = fmt.Appendf(frame, "event: %s\n", ev.Event)
		}
		frame = fmt.Appendf(frame, "data: %s\n\n", ev.Data)
		if req.ResponseBuf != nil {
			_, _ = req.ResponseBuf.Write(frame)
		}
		_, writeErr = w.Write(frame)
		if flusher != nil {
			flusher.Flush()
		}
		return writeErr == nil && errorEvent == nil
	})
	if writeErr != nil {
		return writeErr
	}
	if errorEvent != nil {
		return errorEvent
	}
	if err != nil {
		return err
	}
	if !terminal {
		return proxy.UpstreamFailure(http.StatusBadGateway, "GHE Responses stream ended before response.completed")
	}
	return nil
}
