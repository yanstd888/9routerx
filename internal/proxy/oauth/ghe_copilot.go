package oauth

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"time"

	"9routerx/proxy/internal/models"
	"9routerx/proxy/internal/proxy/ghecopilot"
)

type GHEConnectionStore interface {
	GetProviderConnectionByID(string) (*models.ProviderConnection, error)
	UpdateConnectionData(string, string) error
}

// PrepareGHEConnection shares the existing per-connection renewal mutex, not its
// permanent result cache. Every queued caller reloads the vault-hydrated row.
func PrepareGHEConnection(ctx context.Context, store GHEConnectionStore, client *http.Client, id string, force bool, rejectedToken string) (map[string]any, error) {
	if id == "" {
		return nil, fmt.Errorf("GHE connection id is missing")
	}
	slot := SlotFor(id)
	for !slot.Mu.TryLock() {
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	defer slot.Mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := store.GetProviderConnectionByID(id)
	if err != nil {
		return nil, fmt.Errorf("reload GHE connection: %w", err)
	}
	if conn == nil || conn.Provider != ghecopilot.Provider {
		return nil, fmt.Errorf("connection does not belong to GHE Copilot")
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(conn.Data), &data); err != nil {
		return nil, fmt.Errorf("decode GHE connection: %w", err)
	}
	psd, _ := data["providerSpecificData"].(map[string]any)
	if force && rejectedToken != "" && ghecopilot.String(psd, "copilotToken") != rejectedToken && ghecopilot.Cached(psd, time.Now()) {
		force = false
	}
	changed, renewErr := ghecopilot.Renew(ctx, client, data, force)
	if changed {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("encode GHE credentials: %w", err)
		}
		if err := store.UpdateConnectionData(id, string(raw)); err != nil {
			return nil, fmt.Errorf("persist GHE credentials: %w", err)
		}
	}
	if renewErr != nil {
		return nil, renewErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
