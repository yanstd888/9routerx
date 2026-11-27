package executor

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

// ForwardDevinDesktop executes the native Connect-protobuf Desktop protocol.
// Wire contract: OmniRoute 632d84f5, open-sse/executors/devin-desktop.ts.
func ForwardDevinDesktop(w http.ResponseWriter, req *Request) error {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if req.APIKey == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Devin Desktop API key is required")
	}
	input, ctx, err := devinDesktopInput(req, ctx)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, "invalid Devin Desktop request")
	}
	base := ""
	if req.Config != nil {
		base = req.Config.BaseURL
	}
	if req.Endpoint != "" {
		base = req.Endpoint
	}
	ctx, authURL, err := devinDesktopURL(ctx, base, devinDesktopAuthPath)
	if err != nil {
		return err
	}
	ctx, chatURL, err := devinDesktopURL(ctx, base, devinDesktopChatPath)
	if err != nil {
		return err
	}
	client := devinDesktopClient(req.Client)
	metadata := devinDesktopMetadata{apiKey: req.APIKey, sessionID: input.sessionID}
	auth, err := devinDesktopPost(ctx, client, authURL, wsMsg(1, metadata.encode()), false)
	if err != nil {
		return err
	}
	defer func() { _ = auth.Body.Close() }()
	metadata.jwt, err = devinDesktopJWT(ctx, auth)
	if err != nil {
		return err
	}
	payload := input.encode(metadata)
	if len(payload) > devinDesktopFrameLimit {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Devin Desktop request exceeds the safety limit")
	}
	upstream, err := devinDesktopPost(ctx, client, chatURL, devinDesktopEnvelope(payload, 0), true)
	if err != nil {
		return err
	}
	defer func() { _ = upstream.Body.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = upstream.Body.Close() })
	defer stop()
	local := *req
	local.Ctx = ctx
	local.TranslateResp = req.ClaudeClient || req.TranslateResp
	output := newDevinDesktopOutput(input.model)
	if req.IsStream {
		return devinDesktopStream(w, &local, upstream.Body, upstream.Header.Get("Connect-Content-Encoding"), output)
	}
	err = devinDesktopFrames(ctx, upstream.Body, upstream.Header.Get("Connect-Content-Encoding"), output.collect)
	if err != nil {
		return devinDesktopFailure(ctx, "stream", err)
	}
	body, err := output.json()
	if err != nil {
		return fmt.Errorf("devin desktop completion: %w", err)
	}
	if translator.IsResponsesClient(ctx) {
		return devinDesktopResponsesJSON(ctx, w, &local, body)
	}
	return jsonResponse(ctx, w, bytes.NewReader(body), local.TranslateResp, req.ResponseBuf)
}
