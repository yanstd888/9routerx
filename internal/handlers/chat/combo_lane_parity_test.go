// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package chat

import (
	"reflect"
	"sort"
	"testing"

	"9routerx/proxy/internal/db"
)

// TestRouteComboModelsParity pins the shared dispatcher used by both the chat
// and messages/responses lanes. Each case compares the helper with the strategy
// implementation it is supposed to call, using fresh state so ranking counters
// cannot make the comparison flaky.
func TestRouteComboModelsParity(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`)
	models := []string{"a", "b", "c"}
	strategies := []string{"smart", "tokenmaxxing", "cache-optimized", "auto", "quota-weighted", "reset-aware", "lkgp"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			actualDB, actualCleanup := setupChatTestDB(t)
			defer actualCleanup()
			wantDB, wantCleanup := setupChatTestDB(t)
			defer wantCleanup()

			actual := NewChatHandler(db.NewRepo(actualDB))
			want := NewChatHandler(db.NewRepo(wantDB))
			ctx := t.Context()

			_, got := actual.routeComboModels(ctx, body, models, strategy, "parity-combo", 1)

			var wantModels []string
			switch strategy {
			case "smart":
				_, wantModels = want.applySmartRouting(ctx, body, models, "parity-combo")
			case "tokenmaxxing":
				wantModels = want.applyTokenmaxxingRouting(ctx, models)
			case "cache-optimized":
				wantModels = want.applyCacheOptimizedRouting(body, models)
			case "auto":
				wantModels = want.applyAutoRouting(ctx, body, models)
			case "quota-weighted":
				wantModels = want.applyQuotaWeightedRouting(ctx, models)
			case "reset-aware":
				wantModels = want.applyResetAwareRouting(ctx, models)
			case "lkgp":
				wantModels = want.applyLKGPRouting(ctx, models)
			default:
				t.Fatalf("unhandled strategy %q", strategy)
			}

			// reset-aware scores against the current clock, so two calls a few
			// milliseconds apart may rank equal candidates differently. The
			// dispatcher parity is still pinned by the member set.
			if strategy == "reset-aware" {
				sort.Strings(got)
				sort.Strings(wantModels)
			}
			if !reflect.DeepEqual(got, wantModels) {
				t.Errorf("routeComboModels(%s) = %v, want %v", strategy, got, wantModels)
			}
		})
	}
}
