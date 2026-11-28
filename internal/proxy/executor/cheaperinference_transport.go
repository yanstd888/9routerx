// 9RouterX — CI payload transport.
// OmniRoute 632d84f5: utils/ciPayloadTransport.ts. Lossless transport, never
// retries inference or changes the prompt/model:
//
//   - JSON bodies below 32KiB go out as-is.
//   - Bodies from 32KiB up are gzipped (Content-Encoding: gzip) — CI accepts
//     gzipped JSON on the wire.
//   - When the compressed wire size exceeds 4MiB, the payload goes through
//     CI's same-origin chunked payload-upload reservation: POST
//     /payload-uploads with the endpoint/size/sha256 manifest, PUT 1MiB parts
//     under the reservation, then EXACTLY ONE inference dispatch carrying
//     x-ci-payload-upload with a "{}" body. A failed dispatch deletes the
//     reservation; a lost response must not cause a hidden second generation.
//     Never follow a returned upload URL — only the opaque reservation id is
//     used.
//
// cinfPrepare performs every pre-dispatch step and returns the config/body to
// send, so each caller performs exactly one inference dispatch.
package executor

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"9routerx/proxy/internal/log"
	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

const (
	cinfWireBodyBytes  = 4_000_000
	cinfPayloadPart    = 1024 * 1024
	cinfMaxJSONBytes   = 20 * 1024 * 1024
	cinfMinGzipBytes   = 32 * 1024
	cinfCleanupTimeout = 2 * time.Second // mirrors the pinned DELETE deadline
)

var cinfInferencePath = regexp.MustCompile(`/(chat/completions|responses)$`)

// cinfReservation is the subset of CI's reservation reply the transport uses.
type cinfReservation struct {
	ID       string `json:"id"`
	PartSize int64  `json:"part_bytes"`
}

// cinfForwardChat dispatches through the CI transport and delegates the reply
// to the shared OpenAI response writer.
func cinfForwardChat(w http.ResponseWriter, req *Request, cfg *providers.ProviderConfig, body []byte) error {
	sendCfg, sendBody, guard, err := cinfPrepare(req, cfg, body)
	if err != nil {
		guard.Cleanup()
		return err
	}
	defer guard.Cleanup()
	req.Config = sendCfg
	req.Body = sendBody
	err = ForwardOpenAI(w, req)
	if err == nil {
		guard.MarkOK()
	}
	return err
}

// cinfForwardResponses dispatches through the CI transport and hands the
// Responses-shaped reply to the Codex stream handler, like the pinned
// executor delegates both surfaces to BaseExecutor recovery.
func cinfForwardResponses(w http.ResponseWriter, req *Request, cfg *providers.ProviderConfig, body []byte) error {
	sendCfg, sendBody, guard, err := cinfPrepare(req, cfg, body)
	if err != nil {
		guard.Cleanup()
		return err
	}
	defer guard.Cleanup()
	resp, err := proxy.ForwardOpenAI(reqCtx(req), req.Client, sendCfg, req.APIKey, sendBody, req.IsStream)
	if err != nil {
		return fmt.Errorf("ForwardCheaperInference (responses): %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		return &proxy.UpstreamError{StatusCode: resp.StatusCode, Body: errBody}
	}
	guard.MarkOK()
	if req.IsStream {
		stallReader := proxy.NewStallReaderWithContext(reqCtx(req), resp.Body, sendCfg.ResolveStallTimeout(proxy.ResolveDefaultStallTimeout()), "cheaperinference-responses")
		return handleCodexStream(w, req, stallReader)
	}
	return handleCodexStream(w, req, resp.Body)
}

// cinfUploadCleanup tracks a reserved chunked upload. Cleanup deletes the
// reserved bytes only when the dispatch has not succeeded (needsCleanup = !
// response.ok in the pin): CI drops stored bytes before starting inference,
// so a successful stream is never delayed on DELETE, and a lost response must
// not cause a hidden second generation.
type cinfUploadCleanup struct {
	done     bool
	resource string
	ctx      context.Context
	client   *http.Client
	headers  map[string]string
}

// MarkOK records that the single inference dispatch succeeded, so the
// reserved bytes are CI's to drop rather than ours to delete. A nil guard is
// a non-chunked dispatch: nothing to mark.
func (g *cinfUploadCleanup) MarkOK() {
	if g == nil {
		return
	}
	g.done = true
}

// Cleanup best-effort deletes the reservation; the receiver's retention
// worker reaps abandoned uploads regardless.
func (g *cinfUploadCleanup) Cleanup() {
	if g == nil || g.done {
		return
	}
	cinfCleanupUpload(g.ctx, g.client, g.resource, g.headers)
}

// cinfPrepare runs the pre-dispatch transport steps: size refusal, gzip, and
// the chunked reservation protocol. The returned guard is non-nil only after
// a reserved upload.
func cinfPrepare(req *Request, cfg *providers.ProviderConfig, body []byte) (*providers.ProviderConfig, []byte, *cinfUploadCleanup, error) {
	if len(body) > cinfMaxJSONBytes {
		return nil, nil, nil, proxy.UpstreamFailure(http.StatusRequestEntityTooLarge,
			"CI accepts at most 20 MiB of JSON per request, including chunked uploads.")
	}
	if len(body) < cinfMinGzipBytes {
		return cfg, body, nil, nil
	}
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CI gzip: %w", err)
	}
	if _, err := writer.Write(body); err != nil {
		return nil, nil, nil, fmt.Errorf("CI gzip: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("CI gzip: %w", err)
	}
	compressed := buf.Bytes()
	if len(compressed) <= cinfWireBodyBytes {
		gzipped := *cfg
		gzipped.StaticHeaders = cinfWithHeader(cfg.StaticHeaders, "Content-Encoding", "gzip")
		return &gzipped, compressed, nil, nil
	}
	return cinfReserveAndPrepare(req, cfg, body, compressed)
}

// cinfReserveAndPrepare runs the chunked reservation protocol and returns the
// single-inference-dispatch config ("{}" body carrying the reservation id).
func cinfReserveAndPrepare(req *Request, cfg *providers.ProviderConfig, decoded, compressed []byte) (*providers.ProviderConfig, []byte, *cinfUploadCleanup, error) {
	inferenceURL := strings.TrimRight(cfg.BaseURL, "/")
	match := cinfInferencePath.FindStringSubmatch(inferenceURL)
	if match == nil {
		return nil, nil, nil, proxy.UpstreamFailure(http.StatusBadGateway, "CI inference URL must end in /chat/completions or /responses")
	}
	uploadBase := strings.TrimSuffix(inferenceURL, match[0]) + "/payload-uploads"
	digest := sha256.Sum256(compressed)
	manifest, err := json.Marshal(map[string]any{
		"endpoint":           match[1],
		"encoding":           "gzip",
		"size_bytes":         len(compressed),
		"decoded_size_bytes": len(decoded),
		"sha256":             hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CI reservation manifest: %w", err)
	}
	ctx := reqCtx(req)
	headers := cinfBaseHeaders(cfg, req.APIKey)
	created, err := proxy.DoRequest(ctx, req.Client, http.MethodPost, uploadBase, headers, manifest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CI payload reservation: %w", err)
	}
	defer func() { _ = created.Body.Close() }()
	raw, _ := proxy.ReadBody(created)
	if created.StatusCode < 200 || created.StatusCode >= 300 {
		return nil, nil, nil, &proxy.UpstreamError{StatusCode: created.StatusCode, Body: raw}
	}
	var reservation cinfReservation
	if json.Unmarshal(raw, &reservation) != nil || !cinfValidReservationID(reservation.ID) || reservation.PartSize != cinfPayloadPart {
		return nil, nil, nil, fmt.Errorf("CI returned an invalid payload upload reservation")
	}
	guard := &cinfUploadCleanup{resource: uploadBase + "/" + reservation.ID, ctx: ctx, client: req.Client, headers: headers}
	partHeaders := cinfBaseHeaders(cfg, req.APIKey)
	partHeaders["Content-Type"] = "application/octet-stream"
	for offset, part := 0, 0; offset < len(compressed); offset, part = offset+cinfPayloadPart, part+1 {
		end := offset + cinfPayloadPart
		if end > len(compressed) {
			end = len(compressed)
		}
		uploaded, err := proxy.DoRequest(ctx, req.Client, http.MethodPut, guard.resource+"/parts/"+strconv.Itoa(part), partHeaders, compressed[offset:end])
		if err != nil {
			return nil, nil, guard, fmt.Errorf("CI payload part %d: %w", part, err)
		}
		if uploaded.StatusCode < 200 || uploaded.StatusCode >= 300 {
			body, _ := proxy.ReadBody(uploaded)
			return nil, nil, guard, &proxy.UpstreamError{StatusCode: uploaded.StatusCode, Body: body}
		}
		_ = uploaded.Body.Close()
	}
	dispatched := *cfg
	dispatched.StaticHeaders = cinfWithHeader(cfg.StaticHeaders, "X-CI-Payload-Upload", reservation.ID)
	return &dispatched, []byte("{}"), guard, nil
}

// cinfBaseHeaders builds the auth + JSON headers shared by every CI hop.
func cinfBaseHeaders(cfg *providers.ProviderConfig, apiKey string) map[string]string {
	headers := map[string]string{"Content-Type": "application/json", "Accept": "*/*"}
	if cfg.AuthScheme == "bearer" && apiKey != "" {
		headers[cfg.AuthHeader] = "Bearer " + apiKey
	}
	return headers
}

func cinfWithHeader(static map[string]string, key, value string) map[string]string {
	merged := make(map[string]string, len(static)+1)
	for k, v := range static {
		merged[k] = v
	}
	merged[key] = value
	return merged
}

var cinfReservationIDShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func cinfValidReservationID(id string) bool { return cinfReservationIDShape.MatchString(id) }

// cinfCleanupUpload best-effort deletes reserved bytes independent of caller
// cancellation; the receiver's retention worker reaps abandoned uploads.
func cinfCleanupUpload(ctx context.Context, client *http.Client, resource string, headers map[string]string) {
	cleanupCtx, cancel := context.WithTimeout(ctx, cinfCleanupTimeout)
	defer cancel()
	deleteHeaders := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		deleteHeaders[k] = v
	}
	deleteHeaders["Content-Type"] = "application/json"
	cleaned, err := proxy.DoRequest(cleanupCtx, client, http.MethodDelete, resource, deleteHeaders, []byte("{}"))
	if err != nil {
		log.Warn("cheaperinference", "payload reservation cleanup failed", "resource", resource, "error", err)
		return
	}
	_ = cleaned.Body.Close()
}
