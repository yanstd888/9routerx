package executor

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/url"
	"time"

	"9routerx/proxy/internal/proxy"
)

type opencodeServerMessageBody struct {
	Model  map[string]string `json:"model"`
	System string            `json:"system"`
	Tools  map[string]any    `json:"tools"`
	Parts  []map[string]any  `json:"parts"`
}

type opencodeServerMessageInfo struct {
	ID        string                    `json:"id"`
	Role      string                    `json:"role"`
	SessionID string                    `json:"sessionID"`
	Tokens    opencodeServerTokenCounts `json:"tokens"`
}

type opencodeServerMessageResponse struct {
	Info  opencodeServerMessageInfo `json:"info"`
	Parts []struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		State     string `json:"state"`
		CallID    string `json:"callID"`
		Name      string `json:"name"`
		Input     any    `json:"input"`
		Output    string `json:"output"`
		Mime      string `json:"mime"`
		SessionID string `json:"sessionID"`
		MessageID string `json:"messageID"`
	} `json:"parts"`
}

func opencodeServerSessionPath(sessionID, suffix string) string {
	path := "/session/" + url.PathEscape(sessionID)
	if suffix != "" {
		path += "/" + suffix
	}
	return path
}

func opencodeServerCreateSession(ctx context.Context, client *http.Client, base, title string) (string, error) {
	body, status, err := opencodeServerDo(ctx, client, http.MethodPost, base+"/session", map[string]string{"title": title})
	if err != nil {
		return "", proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", proxy.UpstreamFailure(status, "OpenCode CLI Server could not create a session")
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &created) != nil || created.ID == "" {
		return "", proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode CLI Server session has no id")
	}
	return created.ID, nil
}

func opencodeServerBlockingMessage(ctx context.Context, client *http.Client, base, sessionID string, body opencodeServerMessageBody) (opencodeServerMessageResponse, error) {
	var result opencodeServerMessageResponse
	raw, status, err := opencodeServerDo(ctx, client, http.MethodPost, base+opencodeServerSessionPath(sessionID, "message"), body)
	if err != nil {
		return result, proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return result, proxy.UpstreamFailure(status, "OpenCode CLI Server message failed")
	}
	if json.Unmarshal(raw, &result) != nil {
		return result, proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode CLI Server message is invalid")
	}
	return result, nil
}

func opencodeServerPromptAsync(ctx context.Context, client *http.Client, base, sessionID string, body opencodeServerMessageBody) error {
	_, status, err := opencodeServerDo(ctx, client, http.MethodPost, base+opencodeServerSessionPath(sessionID, "prompt_async"), body)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if status != http.StatusNoContent && status != http.StatusOK && status != http.StatusAccepted {
		return proxy.UpstreamFailure(status, "OpenCode CLI Server prompt_async failed")
	}
	return nil
}

// opencodeServerCleanup is best effort: an aborted or deleted session must not
// leave work running on the local CLI server, but a cleanup failure never
// replaces the answer already on its way to the client.
func opencodeServerCleanup(client *http.Client, base, sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, _ = opencodeServerDo(ctx, client, http.MethodPost, base+opencodeServerSessionPath(sessionID, "abort"), map[string]any{})
	_, _, _ = opencodeServerDo(ctx, client, http.MethodDelete, base+opencodeServerSessionPath(sessionID, ""), nil)
}
