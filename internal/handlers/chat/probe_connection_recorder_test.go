// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package chat

import (
	"testing"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlerutil"
)

func TestProbeConnectionRecorderGetsResolvedConnection(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	h := NewChatHandler(db.NewRepo(database))

	recorder := &handlerutil.ProbeConnectionRecorder{}
	ctx := handlerutil.WithProbeConnectionRecorder(t.Context(), recorder)
	conn, _, err := h.GetBestConnectionWithContext(ctx, "lm-studio", "", nil, "fixture-model")
	if err != nil {
		t.Fatalf("resolve connection: %v", err)
	}
	if conn == nil {
		t.Fatal("resolver returned no connection")
	}
	if got := recorder.ID(); got != conn.ID {
		t.Errorf("recorded connection = %q, want %q", got, conn.ID)
	}
}
