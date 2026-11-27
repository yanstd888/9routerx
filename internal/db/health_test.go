// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func setupHealthConnTestDB(t *testing.T) (*Repo, func()) {
	t.Helper()

	database, cleanup := setupTestDB(t)

	_, err := database.Exec(`CREATE TABLE IF NOT EXISTS providerConnections (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		authType TEXT,
		name TEXT,
		email TEXT,
		priority INTEGER,
		isActive INTEGER DEFAULT 1,
		data TEXT NOT NULL DEFAULT '{}',
		createdAt TEXT NOT NULL DEFAULT (datetime('now')),
		updatedAt TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		cleanup()
		t.Fatalf("create providerConnections: %v", err)
	}
	if err := EnsureAdditiveColumns(database); err != nil {
		cleanup()
		t.Fatalf("additive columns: %v", err)
	}

	repo := NewRepo(database)
	return repo, cleanup
}

func insertTestConn(t *testing.T, repo *Repo) {
	t.Helper()
	_, err := repo.RawDB().Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'api_key', ?, 1, '{}', datetime('now'), datetime('now'))`,
		"conn-1", "openai", "test-conn",
	)
	if err != nil {
		t.Fatalf("insert connection: %v", err)
	}
}

func insertTestConns(t *testing.T, repo *Repo, conns ...string) {
	t.Helper()
	for i, id := range conns {
		_, err := repo.RawDB().Exec(
			`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'api_key', ?, 1, '{}', datetime('now'), datetime('now'))`,
			id, "openai", id,
		)
		if err != nil {
			t.Fatalf("insert connection %d: %v", i, err)
		}
	}
}

func TestIsProviderHealthy_NoConnections(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	if !IsProviderHealthy(repo.RawDB(), "openai", "gpt-4") {
		t.Error("expected healthy=true when no connections exist")
	}
}

func TestIsProviderHealthy_NoModelLock(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	if !IsProviderHealthy(repo.RawDB(), "openai", "gpt-4") {
		t.Error("expected healthy=true when no model lock exists")
	}
}

func TestIsProviderHealthy_WithModelLock(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	err := repo.LockConnectionModel("conn-1", "gpt-4", 3600, 1)
	if err != nil {
		t.Fatalf("LockConnectionModel: %v", err)
	}

	if IsProviderHealthy(repo.RawDB(), "openai", "gpt-4") {
		t.Error("expected healthy=false when all connections have active model lock")
	}
}

func TestIsProviderHealthy_WithUnlockedConnection(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConns(t, repo, "conn-1", "conn-2")

	err := repo.LockConnectionModel("conn-1", "gpt-4", 3600, 1)
	if err != nil {
		t.Fatalf("LockConnectionModel: %v", err)
	}

	if !IsProviderHealthy(repo.RawDB(), "openai", "gpt-4") {
		t.Error("expected healthy=true when at least one connection is unlocked")
	}
}

func TestResetProviderHealth_ClearsModelLocks(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	err := repo.LockConnectionModel("conn-1", "gpt-4", 3600, 1)
	if err != nil {
		t.Fatalf("LockConnectionModel: %v", err)
	}

	err = repo.ResetProviderHealth("openai", "gpt-4")
	if err != nil {
		t.Fatalf("ResetProviderHealth: %v", err)
	}

	if !IsProviderHealthy(repo.RawDB(), "openai", "gpt-4") {
		t.Error("expected healthy=true after resetting health")
	}
}

// TestResetProviderHealth_ClearsDottedModelLocks: the lock for a model whose
// name contains a dot (gpt-3.5-turbo, gemini-2.5-pro, …) is stored as one flat
// key via modelLockDataKey's quoted path, but the clear paths built an
// unquoted "$."+key path, which SQLite reads as a nested path. The reset then
// missed the lock (the model stayed unavailable until its lock expired) and
// wrote junk nested keys into the connection data blob.
func TestResetProviderHealth_ClearsDottedModelLocks(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	if err := repo.LockConnectionModel("conn-1", "gpt-3.5-turbo", 3600, 1); err != nil {
		t.Fatalf("LockConnectionModel: %v", err)
	}
	if repo.IsProviderAvailable("openai", "gpt-3.5-turbo") {
		t.Fatal("expected available=false while the lock is held")
	}

	if err := repo.ResetProviderHealth("openai", "gpt-3.5-turbo"); err != nil {
		t.Fatalf("ResetProviderHealth: %v", err)
	}

	if !repo.IsProviderAvailable("openai", "gpt-3.5-turbo") {
		t.Error("expected the dotted model lock to be cleared by the reset")
	}

	// The failed clear used to leave junk like {"modelLock_gpt-3":{"5-turbo":null}}
	// behind; every modelLock_* key in the blob must stay a scalar (null or a
	// timestamp string), never a nested object.
	var data string
	if err := repo.RawDB().QueryRow(`SELECT data FROM providerConnections WHERE id = 'conn-1'`).Scan(&data); err != nil {
		t.Fatalf("read connection data: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatalf("decode connection data: %v", err)
	}
	for k, v := range decoded {
		if strings.HasPrefix(k, modelLockPrefix) {
			if _, nested := v.(map[string]any); nested {
				t.Errorf("reset polluted the data blob with a nested junk key %q: %s", k, data)
			}
		}
	}
}

// TestResetProviderHealth_ClearsAllDottedLocks covers the provider-wide arm of
// the reset, which walks every connection's stored lock keys.
func TestResetProviderHealth_ClearsAllDottedLocks(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	if err := repo.LockConnectionModel("conn-1", "gemini-2.5-pro", 3600, 1); err != nil {
		t.Fatalf("LockConnectionModel: %v", err)
	}

	if err := repo.ResetProviderHealth("openai", ""); err != nil {
		t.Fatalf("ResetProviderHealth: %v", err)
	}

	if !repo.IsProviderAvailable("openai", "gemini-2.5-pro") {
		t.Error("expected the dotted model lock to be cleared by the provider-wide reset")
	}
}

func TestIsProviderAvailable_Repo(t *testing.T) {
	repo, cleanup := setupHealthConnTestDB(t)
	defer cleanup()

	insertTestConn(t, repo)

	if !repo.IsProviderAvailable("openai", "gpt-4") {
		t.Error("expected available=true when no lock")
	}

	repo.LockConnectionModel("conn-1", "gpt-4", 3600, 1)

	if repo.IsProviderAvailable("openai", "gpt-4") {
		t.Error("expected available=false when locked")
	}
}
