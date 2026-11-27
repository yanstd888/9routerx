// 9RouterX — CI media detection and mixed-media normalization.
// OmniRoute 632d84f5: utils/mediaParts.ts (carrier shapes), utils/ciTransportInput.ts
// (fail-closed guard input) and utils/ciMixedMedia.ts (the file-carrier rewrite).
// The detector covers the carriers the CI executor acts on — audio, video and
// document (pdf) parts plus data-URI strings — with the same bounded traversal
// (depth 8) as the pin; image parts are detected for the kind-set only.
package executor

import (
	"encoding/json"
	"strconv"
	"strings"
)

const cinfMaxDepth = 8

// cinfMediaKinds is the distinct set of media kinds found in a request body.
type cinfMediaKinds map[string]bool

// cinfDetectMediaKinds walks the chat/messages shapes the pin scans
// (messages[], input, contents) and collects the media kinds present. Only
// kind presence matters here — no refs are collected.
func cinfDetectMediaKinds(body []byte) cinfMediaKinds {
	var probe struct {
		Messages json.RawMessage `json:"messages"`
		Input    json.RawMessage `json:"input"`
		Contents json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return nil
	}
	kinds := cinfMediaKinds{}
	var walk func(raw json.RawMessage, depth int)
	walk = func(raw json.RawMessage, depth int) {
		if depth > cinfMaxDepth+2 || len(raw) == 0 {
			return
		}
		// Strings are checked for data-URI media kinds.
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if kind := cinfDataURIKind(s); kind != "" {
				kinds[kind] = true
			}
			return
		}
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil {
			for _, item := range arr {
				walk(item, depth+1)
			}
			return
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return
		}
		kind, stop := cinfInspectObject(obj)
		if kind != "" {
			kinds[kind] = true
		}
		if stop {
			return
		}
		for _, nested := range obj {
			walk(nested, depth+1)
		}
	}
	for _, field := range []json.RawMessage{probe.Messages, probe.Input, probe.Contents} {
		var arr []json.RawMessage
		if json.Unmarshal(field, &arr) == nil {
			for _, message := range arr {
				var m struct {
					Content json.RawMessage `json:"content"`
				}
				if json.Unmarshal(message, &m) != nil || len(m.Content) == 0 {
					continue
				}
				var parts []json.RawMessage
				if json.Unmarshal(m.Content, &parts) != nil {
					continue
				}
				for _, part := range parts {
					walk(part, 0)
				}
			}
			continue
		}
		// input/contents may be single objects or arrays of content parts
		// (Responses shape) rather than message objects; scan them directly.
		walk(field, 0)
	}
	return kinds
}

// cinfInspectObject classifies one content-part object. The second return is
// true when the object was consumed (a part was pushed) so nested walking
// stops, mirroring the pin's early returns.
func cinfInspectObject(obj map[string]json.RawMessage) (string, bool) {
	var typeField string
	if raw, ok := obj["type"]; ok {
		json.Unmarshal(raw, &typeField)
	}
	// input_audio: {type:"input_audio", input_audio:{data, format}}
	if typeField == "input_audio" {
		var audio map[string]json.RawMessage
		if json.Unmarshal(obj["input_audio"], &audio) == nil {
			if data, ok := audio["data"]; ok && len(data) > 0 && string(data) != "null" {
				return "audio", true
			}
		}
	}
	// audio_url: {type:"audio_url", audio_url:"..."|{url}}
	if typeField == "audio_url" {
		if cinfURLRef(obj["audio_url"]) != "" {
			return "audio", true
		}
	}
	// video carriers: input_video / video_url
	if typeField == "video_url" || typeField == "input_video" {
		if cinfURLRef(obj["video_url"]) != "" || cinfURLRef(obj["input_video"]) != "" || cinfURLRef(obj["url"]) != "" {
			return "video", true
		}
	}
	// source-based shapes: Anthropic-style {source:{media_type, data|url}}
	var source map[string]json.RawMessage
	if json.Unmarshal(obj["source"], &source) == nil {
		var mediaType string
		if raw, ok := source["media_type"]; ok {
			json.Unmarshal(raw, &mediaType)
		}
		mediaType = strings.ToLower(mediaType)
		if strings.HasPrefix(mediaType, "audio/") {
			if data, ok := source["data"]; ok && len(data) > 0 && string(data) != "null" {
				return "audio", true
			}
		}
		if strings.HasPrefix(mediaType, "video/") {
			if data, ok := source["data"]; ok && len(data) > 0 && string(data) != "null" {
				return "video", true
			}
			if cinfURLRef(source["url"]) != "" {
				return "video", true
			}
		}
	}
	// file/document attachments: {type:"file"|"document", file:{filename,file_data}|{file_data}} or
	// {type:"document", source:{media_type:"application/pdf", data}}
	if typeField == "file" || typeField == "document" {
		var file map[string]json.RawMessage
		if json.Unmarshal(obj["file"], &file) == nil {
			var filename, fileData string
			json.Unmarshal(file["filename"], &filename)
			json.Unmarshal(file["file_data"], &fileData)
			if strings.Contains(strings.ToLower(fileData), "application/pdf") || strings.HasSuffix(strings.ToLower(filename), ".pdf") {
				return "document", true
			}
		}
		var pdfSource map[string]json.RawMessage
		if json.Unmarshal(obj["source"], &pdfSource) == nil {
			var mediaType string
			json.Unmarshal(pdfSource["media_type"], &mediaType)
			if strings.EqualFold(mediaType, "application/pdf") {
				return "document", true
			}
		}
	}
	return "", false
}

// cinfURLRef extracts a URL from a bare string or {url:"..."} field.
func cinfURLRef(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var holder struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &holder) == nil {
		return holder.URL
	}
	return ""
}

// cinfDataURIKind classifies a data-URI string the way the pin's
// dataUriKind does for the kinds the CI executor consumes.
func cinfDataURIKind(value string) string {
	if !strings.HasPrefix(value, "data:") {
		return ""
	}
	head := value
	if idx := strings.IndexByte(head, ','); idx >= 0 {
		head = head[:idx]
	}
	head = strings.ToLower(head)
	switch {
	case strings.Contains(head, "audio/"):
		return "audio"
	case strings.Contains(head, "video/"):
		return "video"
	case strings.Contains(head, "application/pdf"):
		return "document"
	}
	return ""
}

// cinfUnsupportedMediaKind reports the first media kind the model has no
// verified transport for, or "" when every present kind is accepted.
func cinfUnsupportedMediaKind(model string, body []byte) string {
	kinds := cinfDetectMediaKinds(body)
	for _, kind := range []string{"audio", "document", "video"} {
		if kinds[kind] && !cinfTransportSupportsInput(model, kind) {
			// The pin names the client-facing kinds "audio", "pdf", "video".
			label := kind
			if kind == "document" {
				label = "pdf"
			}
			return label
		}
	}
	return ""
}

// cinfNormalizeMixedMedia mirrors normalizeCIMixedMedia: when the request
// carries two or more distinct media kinds, inline audio and video carriers
// are rewritten into CI's MIME-typed file carrier (no re-encoding); with a
// single kind the body is returned untouched.
func cinfNormalizeMixedMedia(body []byte) []byte {
	kinds := cinfDetectMediaKinds(body)
	if len(kinds) < 2 {
		return body
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil {
		return body
	}
	index := 0
	changed := false
	for _, key := range []string{"messages", "input", "contents"} {
		raw, ok := root[key]
		if !ok {
			continue
		}
		rewritten, did := cinfRewriteCarriers(raw, &index)
		if did {
			root[key] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return out
}

// cinfRewriteCarriers walks bounded content converting input_audio and
// base64-data-URI video_url parts into {type:"file", file:{filename,file_data}}.
func cinfRewriteCarriers(raw json.RawMessage, index *int) (json.RawMessage, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return raw, false
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		changed := false
		out := make([]json.RawMessage, len(arr))
		for i, item := range arr {
			out[i], _ = cinfRewriteCarriers(item, index)
			if string(out[i]) != string(item) {
				changed = true
			}
		}
		if !changed {
			return raw, false
		}
		merged, err := json.Marshal(out)
		if err != nil {
			return raw, false
		}
		return merged, true
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw, false
	}
	var typeField string
	json.Unmarshal(obj["type"], &typeField)
	if typeField == "input_audio" {
		var audio map[string]json.RawMessage
		if json.Unmarshal(obj["input_audio"], &audio) == nil {
			var data, format string
			json.Unmarshal(audio["data"], &data)
			json.Unmarshal(audio["format"], &format)
			format = strings.ToLower(format)
			if mime := cinfAudioMIME(format); mime != "" && data != "" && !strings.HasPrefix(data, "data:") && len(audio) == 2 {
				*index++
				replacement := map[string]any{
					"type": "file",
					"file": map[string]string{
						"filename": "audio-" + strconv.Itoa(*index) + "." + format,
						"file_data": "data:" + mime + ";base64," + data,
					},
				}
				// Retain unrecognized sibling fields (the pin spreads ...rest).
				for k, v := range obj {
					if k == "type" || k == "input_audio" {
						continue
					}
					var anyVal any
					if json.Unmarshal(v, &anyVal) == nil {
						replacement[k] = anyVal
					}
				}
				merged, err := json.Marshal(replacement)
				if err == nil {
					return merged, true
				}
			}
		}
	}
	if typeField == "video_url" {
		url := cinfURLRef(obj["video_url"])
		if ext := cinfVideoExtension(url); ext != "" && len(obj) == 2 {
			*index++
			replacement := map[string]any{
				"type": "file",
				"file": map[string]string{
					"filename": "video-" + strconv.Itoa(*index) + "." + ext,
					"file_data": url,
				},
			}
			merged, err := json.Marshal(replacement)
			if err == nil {
				return merged, true
			}
		}
	}
	changed := false
	for key, nested := range obj {
		rewritten, did := cinfRewriteCarriers(nested, index)
		if did {
			obj[key] = rewritten
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	merged, err := json.Marshal(obj)
	if err != nil {
		return raw, false
	}
	return merged, true
}

// cinfAudioMIME mirrors the pinned AUDIO_MIME table.
func cinfAudioMIME(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "webm":
		return "audio/webm"
	case "m4a", "mp4":
		return "audio/mp4"
	}
	return ""
}

// cinfVideoExtension mirrors the pinned VIDEO_EXTENSION table for base64
// data-URI video carriers.
func cinfVideoExtension(url string) string {
	const marker = ";base64,"
	if !strings.HasPrefix(strings.ToLower(url), "data:video/") || !strings.Contains(url, marker) {
		return ""
	}
	head := strings.ToLower(url[:strings.Index(url, marker)])
	mime := strings.TrimPrefix(head, "data:")
	switch mime {
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "video/mpeg":
		return "mpeg"
	case "video/x-matroska":
		return "mkv"
	}
	return ""
}
