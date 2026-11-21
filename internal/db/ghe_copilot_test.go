package db

import (
	json "encoding/json/v2"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/vault"
)

func TestGHEDerivedTokenVaultAndOAuthIsolation(t *testing.T) {
	database, cleanup := setupTestDBAt(t, filepath.Join(t.TempDir(), "ghe.sqlite"))
	defer cleanup()
	repo := NewRepo(database)
	v, err := vault.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	repo.SetVault(v)
	if _, err := database.Exec(`INSERT INTO providerConnections(id,provider,authType,isActive,data,createdAt,updatedAt) VALUES('ghe-vault','ghe-copilot','oauth',1,'{}',?,?)`, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	data := `{"accessToken":"oauth-secret","refreshToken":"refresh-secret","apiKey":"independent-secret","alias":"keep","providerSpecificData":{"gheUrl":"https://enterprise.example","copilotToken":"derived-secret","copilotApiUrl":"https://api.enterprise.example"}}`
	if err := repo.UpdateConnectionData("ghe-vault", data); err != nil {
		t.Fatal(err)
	}
	raw := rawConnectionData(t, repo, "ghe-vault")
	for _, secret := range []string{"oauth-secret", "refresh-secret", "independent-secret", "derived-secret"} {
		if strings.Contains(raw, secret) {
			t.Errorf("secret at rest %s", secret)
		}
	}
	conn, err := repo.GetProviderConnectionByID("ghe-vault")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(conn.Data), &fields); err != nil {
		t.Fatal(err)
	}
	psd, _ := fields["providerSpecificData"].(map[string]any)
	psd["copilotToken"] = "renewed-derived"
	updated, _ := json.Marshal(fields)
	if err := repo.UpdateConnectionData("ghe-vault", string(updated)); err != nil {
		t.Fatal(err)
	}
	conn, err = repo.GetProviderConnectionByID("ghe-vault")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"oauth-secret", "refresh-secret", "independent-secret", "renewed-derived", "keep"} {
		if !strings.Contains(conn.Data, secret) {
			t.Errorf("lost hydrated field %s: %s", secret, conn.Data)
		}
	}
	if strings.Contains(rawConnectionData(t, repo, "ghe-vault"), "renewed-derived") {
		t.Fatal("rotated derived token unsealed")
	}
}
