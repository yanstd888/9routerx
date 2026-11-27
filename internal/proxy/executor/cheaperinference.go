// 9RouterX — native executor for Cheaper Inference (api.cheaperinference.com).
// OmniRoute 632d84f5: open-sse/executors/cheaperinference.ts,
// utils/ciTransportInput.ts, utils/ciPayloadTransport.ts and
// utils/ciMixedMedia.ts. The gateway is OpenAI-compatible on both surfaces;
// three pinned facts shape this port:
//
//  1. Chat and Responses live at DIFFERENT URLs. The per-model targetFormat
//     tag (openai-responses on the six GPT-5.x models) decides the surface;
//     /v1/responses is STATELESS and REQUIRES store:false. buildResponsesBody
//     hard-sets store:false on both of its paths (client store:true is
//     overwritten rather than forwarded — the endpoint cannot honour it).
//
//  2. Media transports are fail-closed (ciTransportSupportsInput, verified
//     live upstream 2026-11-03): audio input rides only gemini-3.7-flash on
//     this catalog, and no catalog model has a verified pdf/video transport.
//     Requests carrying those parts for an unsupported model are refused with
//     400 before dispatch so the failure never trips provider cooldown.
//
//  3. Large JSON payloads travel losslessly: bodies >=32KiB are gzipped
//     (Content-Encoding: gzip), and bodies whose compressed wire size exceeds
//     4MiB go through CI's same-origin chunked payload-upload reservation
//     (up to 20MiB decoded) with exactly one inference dispatch.
package executor

import (
	"net/http"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const cinfResponsesBase = "https://api.cheaperinference.com/v1/responses"

// cinfResponsesModels mirrors the pinned per-model targetFormat:
// "openai-responses" tags on the GPT-5.x family (registry index.ts).
var cinfResponsesModels = map[string]bool{
	"gpt-5.4":       true,
	"gpt-5.4-mini":  true,
	"gpt-5.5":       true,
	"gpt-5.6-luna":  true,
	"gpt-5.6-sol":   true,
	"gpt-5.6-terra": true,
}

// cinfAudioModels mirrors ciTransportSupportsInput for this provider: audio
// input is verified only on gemini-3.7-flash (gemini-4-argon was verified
// upstream but is not on the pinned catalog). PDF/video have no verified
// transport on any pinned model.
func cinfTransportSupportsInput(model, kind string) bool {
	if kind == "audio" {
		return model == "gemini-3.7-flash"
	}
	return false
}

// cinfModel resolves the vendor id: the router prefix ("cinf/") is stripped
// and the alias bare id is accepted like the pin's id lookups.
func cinfModel(req *Request) string {
	model := strings.TrimSpace(req.ModelName)
	for _, prefix := range []string{"cinf/", "cheaperinference/"} {
		model = strings.TrimPrefix(model, prefix)
	}
	return model
}

// ForwardCheaperInference guards media transports, normalizes mixed inline
// media, routes the model to its pinned surface and hands the payload to the
// CI transport.
func ForwardCheaperInference(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Cheaper Inference provider config missing")
	}
	model := cinfModel(req)
	if kind := cinfUnsupportedMediaKind(model, req.Body); kind != "" {
		return proxy.UpstreamFailure(http.StatusBadRequest,
			"This Cheaper Inference model has no verified "+kind+" input transport. Select a supported model such as gemini-3.7-flash.")
	}
	body := cinfNormalizeMixedMedia(req.Body)

	cfg := *req.Config
	if cinfResponsesModels[model] {
		transformed, _, err := buildResponsesBody(body)
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadRequest, "Cheaper Inference Responses body build failed: "+err.Error())
		}
		// buildResponsesBody hard-sets store:false on both conversion paths,
		// which is exactly the stateless-endpoint contract; a client store
		// value cannot survive it.
		cfg.BaseURL = cinfResponsesURL(cfg.BaseURL)
		return cinfForwardResponses(w, req, &cfg, transformed)
	}
	return cinfForwardChat(w, req, &cfg, body)
}

// cinfResponsesURL derives the Responses surface from the configured chat
// base: same origin, path suffix swapped, so a connection Base URL override
// repoints both surfaces together (the pin resolves the pair from one config).
func cinfResponsesURL(chatBase string) string {
	base := strings.TrimRight(strings.TrimSpace(chatBase), "/")
	if base == "" || !strings.HasSuffix(base, "/chat/completions") {
		return cinfResponsesBase
	}
	return strings.TrimSuffix(base, "/chat/completions") + "/responses"
}
