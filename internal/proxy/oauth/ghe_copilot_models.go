package oauth

import (
	"context"
	json "encoding/json/v2"
	"fmt"
)

// StoreGHEModelFormats uses the credential mutex and reloads before merging so
// model discovery never writes its earlier OAuth/token snapshot over a renewal.
func StoreGHEModelFormats(ctx context.Context, store GHEConnectionStore, id string, formats map[string]any) error {
	return WithRefreshLock(id, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := store.GetProviderConnectionByID(id)
		if err != nil {
			return err
		}
		if conn == nil || conn.Provider != "ghe-copilot" {
			return fmt.Errorf("not a GHE Copilot connection")
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(conn.Data), &data); err != nil {
			return err
		}
		psd, _ := data["providerSpecificData"].(map[string]any)
		if psd == nil {
			return fmt.Errorf("GHE connection metadata missing")
		}
		psd["gheModelFormats"] = formats
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return store.UpdateConnectionData(id, string(raw))
	})
}
