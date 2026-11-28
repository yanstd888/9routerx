// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package executor

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
)

// WorkBuddy requires streaming upstream; ForwardOpenAI folds SSE for clients
// that requested a buffered reply without changing their downstream format.
func ForwardWorkbuddy(w http.ResponseWriter, req *Request) error {
	var body map[string]jsontext.Value
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return fmt.Errorf("workbuddy request: %w", err)
	}
	body["stream"] = jsontext.Value("true")
	wire, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("workbuddy request: %w", err)
	}
	copyReq := *req
	copyReq.Body = wire
	return ForwardOpenAI(w, &copyReq)
}
