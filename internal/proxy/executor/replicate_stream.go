// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
package executor

import (
	"context"
	"net/http"
	"strings"

	json "encoding/json/v2"

	"9routerx/proxy/internal/proxy"
)

func replicateErrorFrame(status int, message string) []byte {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"message": message, "type": "upstream_error", "code": status},
	}, json.Deterministic(true))
	return append([]byte("data: "), append(body, '\n', '\n')...)
}

var replicateDoneFrame = []byte("data: [DONE]\n\n")

func replicateWriteSSE(w http.ResponseWriter, frames ...[]byte) {
	flusher := proxy.WriteSSEHeaders(w)
	for _, frame := range frames {
		_, _ = w.Write(frame)
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// synthesizeReplicateStream emits a finished prediction as one OpenAI stream.
func synthesizeReplicateStream(w http.ResponseWriter, content, model string, usage replicateUsage, finish string) {
	chunks := newReplicateChunks(model)
	frames := [][]byte{chunks.roleFrame()}
	if content != "" {
		frames = append(frames, chunks.contentFrame(content))
	}
	frames = append(frames, chunks.finishFrame(finish), chunks.usageFrame(usage), replicateDoneFrame)
	replicateWriteSSE(w, frames...)
}

// relayReplicateStream translates Replicate's native output/error/done events
// into OpenAI chunks. The stream host gets no credentials: only an Accept and
// a no-store Cache-Control header ride along.
func relayReplicateStream(ctx context.Context, w http.ResponseWriter, client *http.Client, req *Request, streamURL string, created map[string]any, requestURL string, headers map[string]string, built replicateBuilt, model string) error {
	if !isTrustedStreamURL(streamURL, requestURL) {
		return nil
	}
	streamHeaders := map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-store",
	}
	resp, err := replicateDo(ctx, client, http.MethodGet, streamURL, streamHeaders, nil)
	if err != nil {
		writeReplicateStreamFailure(w, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := replicateUpstreamFailure(resp)
		writeReplicateStreamFailure(w, failure)
		return nil
	}

	chunks := newReplicateChunks(model)
	flusher := proxy.WriteSSEHeaders(w)
	write := func(frame []byte) {
		_, _ = w.Write(frame)
		if flusher != nil {
			flusher.Flush()
		}
	}
	write(chunks.roleFrame())

	var content strings.Builder
	var streamError string
	scanErr := proxy.ScanStreamEvents(resp.Body, func(ev proxy.SSEEvent) bool {
		if ctx.Err() != nil {
			return false
		}
		switch ev.Event {
		case "output":
			data := string(ev.Data)
			if data != "" {
				content.WriteString(data)
				write(chunks.contentFrame(data))
			}
		case "error":
			streamError = replicateSanitizeDetail(replicateDetail(ev.Data))
			return false
		case "done":
			return false
		}
		return true
	})
	if ctx.Err() != nil {
		cancelReplicatePrediction(context.Background(), client, created, requestURL, headers)
		return nil
	}
	if streamError == "" && scanErr != nil {
		streamError = "stream interrupted"
	}
	if streamError != "" {
		write(replicateErrorFrame(http.StatusBadGateway, "Replicate prediction failed: "+streamError))
		write(replicateDoneFrame)
		return nil
	}

	final := fetchFinalReplicatePrediction(ctx, client, created, requestURL, headers)
	usage := replicateResolveUsage(final, built.promptText, content.String())
	write(chunks.finishFrame(replicateFinishReason(usage, built.maxTokens)))
	write(chunks.usageFrame(usage))
	write(replicateDoneFrame)
	return nil
}

func writeReplicateStreamFailure(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	message := "Replicate stream failed"
	if failure, ok := err.(*replicateHTTPError); ok {
		status = failure.status
		message = failure.message
	}
	replicateWriteSSE(w, replicateErrorFrame(status, message), replicateDoneFrame)
}
