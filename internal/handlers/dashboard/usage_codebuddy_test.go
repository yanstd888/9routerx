package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

// The CN lane used to have no case in fetchProviderUsage, so every
// codebuddy-cn connection fell through to the "Account active. No quota limits
// tracked." fallback while the intl lane rendered its Monthly / Bonus Pack
// rows. These tests pin both the dispatch and the wire shape of the CN read.

// codebuddyMeterBody is the billing meter envelope both lanes return. The first
// account is a recurring refill (its DeductionEndTime runs far past the cycle
// end); the second is a bonus package that ends before it. Timestamps are RFC3339
// with a zone on purpose: the naive "2006-01-02 15:04:05" form the providers
// actually send parses in time.Local, which would make the assertions depend on
// the machine's timezone.
const codebuddyMeterBody = `{
	"code": 0,
	"data": {
		"Response": {
			"Data": {
				"Accounts": [
					{
						"PackageName": "CodeBuddy Pro",
						"CycleStartTime": "2026-11-01T00:00:00Z",
						"CycleEndTime": "2026-12-01T00:00:00Z",
						"CycleCapacitySizePrecise": "300",
						"CycleCapacityUsedPrecise": "120",
						"DeductionEndTime": 1893456000000
					},
					{
						"PackageName": "Bonus",
						"CycleEndTime": "2026-12-15T00:00:00Z",
						"CapacitySize": 50,
						"CapacityUsed": 10,
						"DeductionEndTime": 1796083200000
					}
				]
			}
		}
	}
}`

// withCodeBuddyMeterServer points a CodeBuddy billing endpoint at a fake
// upstream and restores it afterwards. The returned func reports the request
// the fetcher sent, so a test can assert the identity headers.
func withCodeBuddyMeterServer(t *testing.T, provider string, status int, body string) func() *http.Request {
	t.Helper()
	var seen *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Clone(context.Background())
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer tok")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	// Both endpoints are package vars, so the swap is visible to the fetcher.
	target := &codebuddyIntlUsageURL
	if provider == "codebuddy-cn" {
		target = &codebuddyCNUsageURL
	}
	prev := *target
	*target = srv.URL
	t.Cleanup(func() { *target = prev })
	return func() *http.Request { return seen }
}

func codebuddyRow(t *testing.T, res usageResult, key string) map[string]any {
	t.Helper()
	row, ok := res.quotas[key].(map[string]any)
	if !ok {
		t.Fatalf("expected a %q row, got %v", key, res.quotas)
	}
	return row
}

// The dispatch gap: before the fix this returned ok=false and the panel showed
// only the static "no quota limits tracked" line.
func TestFetchProviderUsage_DispatchesCodeBuddyCN(t *testing.T) {
	withCodeBuddyMeterServer(t, "codebuddy-cn", http.StatusOK, codebuddyMeterBody)

	res, ok := fetchProviderUsage(context.Background(), "codebuddy-cn", map[string]any{"accessToken": "tok"})
	if !ok {
		t.Fatal("expected fetchProviderUsage to handle codebuddy-cn")
	}
	if res.message != "" {
		t.Fatalf("unexpected message: %s", res.message)
	}
	if res.plan != "CodeBuddy Pro" {
		t.Errorf("plan = %q, want %q", res.plan, "CodeBuddy Pro")
	}
	monthly := codebuddyRow(t, res, "Monthly")
	if got := monthly["total"]; got != float64(300) {
		t.Errorf("Monthly total = %v, want 300", got)
	}
	if got := monthly["used"]; got != float64(120) {
		t.Errorf("Monthly used = %v, want 120", got)
	}
	if got := monthly["recurring"]; got != true {
		t.Errorf("Monthly recurring = %v, want true", got)
	}
	if got := monthly["resetAt"]; got != "2026-12-01T00:00:00Z" {
		t.Errorf("Monthly resetAt = %v, want 2026-12-01T00:00:00Z", got)
	}
	bonus := codebuddyRow(t, res, "Bonus Pack 1")
	if got := bonus["total"]; got != float64(50) {
		t.Errorf("Bonus Pack 1 total = %v, want 50", got)
	}
	if got := bonus["recurring"]; got != false {
		t.Errorf("Bonus Pack 1 recurring = %v, want false", got)
	}
}

// The CN read must carry the CLI identity the registry declares for the chat
// lane, not the IDE identity the intl lane uses. X-Domain follows the origin the
// request is actually sent to — the executor overrides the registry's
// www.codebuddy.cn per connection, so deriving it here is what keeps the billing
// read looking like the chat client.
func TestFetchCodeBuddyUsage_CNIdentityHeaders(t *testing.T) {
	seen := withCodeBuddyMeterServer(t, "codebuddy-cn", http.StatusOK, codebuddyMeterBody)

	fetchCodeBuddyUsage(context.Background(), "codebuddy-cn", "tok", "")
	req := seen()
	if req == nil {
		t.Fatal("no request reached the fake billing endpoint")
	}
	checks := map[string]string{
		"X-IDE-Type": "CLI",
		"X-IDE-Name": "CLI",
		"X-Domain":   req.Host,
	}
	for name, want := range checks {
		if got := req.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if ua := req.Header.Get("User-Agent"); !strings.HasPrefix(ua, "CLI/") {
		t.Errorf("User-Agent = %q, want a CLI/ prefix", ua)
	}
}

// The production origin must still announce itself as the CN host, so a
// regression that leaves the registry's www.codebuddy.cn in place — or drops
// X-Domain entirely — cannot pass by matching a loopback host.
func TestCodeBuddyCNHeaders_TrackOrigin(t *testing.T) {
	live := codebuddyCNHeaders("https://copilot.tencent.com/v2/billing/meter/get-user-resource")
	if got := live["X-Domain"]; got != "copilot.tencent.com" {
		t.Errorf("X-Domain = %q, want copilot.tencent.com", got)
	}
	if got := live["X-IDE-Type"]; got != "CLI" {
		t.Errorf("X-IDE-Type = %q, want CLI", got)
	}
	// The registry map is shared by every request; the copy must not write back.
	if got := providers.KnownProviders["codebuddy-cn"].StaticHeaders["Content-Type"]; got != "" {
		t.Errorf("registry StaticHeaders was mutated: Content-Type = %q", got)
	}
}

// The intl lane must keep exactly what it sent before the refactor: the IDE
// identity against the .ai origin.
func TestFetchCodeBuddyUsage_IntlIdentityUnchanged(t *testing.T) {
	seen := withCodeBuddyMeterServer(t, "codebuddy-intl", http.StatusOK, codebuddyMeterBody)

	res := fetchCodeBuddyIntlUsage(context.Background(), "tok", "")
	if res.plan != "CodeBuddy Pro" {
		t.Errorf("plan = %q, want %q", res.plan, "CodeBuddy Pro")
	}
	req := seen()
	if req == nil {
		t.Fatal("no request reached the fake billing endpoint")
	}
	if got := req.Header.Get("X-IDE-Type"); got != "IDE" {
		t.Errorf("X-IDE-Type = %q, want IDE", got)
	}
	if got := req.Header.Get("User-Agent"); got != "IDE/2.108.1 CodeBuddy/2.108.1" {
		t.Errorf("User-Agent = %q, want the IDE identity", got)
	}
	if got := req.Header.Get("X-Domain"); got != "" {
		t.Errorf("X-Domain = %q, want none (intl never sent one)", got)
	}
}

// A rejected credential must name the lane that rejected it, so an operator
// reading "CodeBuddy (codebuddy-cn) credential invalid" does not go hunting in
// the intl account.
func TestFetchCodeBuddyUsage_ErrorNamesProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		status   int
		body     string
		want     string
	}{
		{
			name:     "cn unauthorized",
			provider: "codebuddy-cn",
			status:   http.StatusUnauthorized,
			body:     `{}`,
			want:     "codebuddy-cn",
		},
		{
			name:     "intl unauthorized",
			provider: "codebuddy-intl",
			status:   http.StatusUnauthorized,
			body:     `{}`,
			want:     "codebuddy-intl",
		},
		{
			name:     "cn upstream error code",
			provider: "codebuddy-cn",
			status:   http.StatusOK,
			body:     `{"code":40001,"msg":"token invalid"}`,
			want:     "token invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withCodeBuddyMeterServer(t, tt.provider, tt.status, tt.body)
			res, ok := fetchProviderUsage(context.Background(), tt.provider, map[string]any{"apiKey": "tok"})
			if !ok {
				t.Fatalf("expected fetchProviderUsage to handle %s", tt.provider)
			}
			if !strings.Contains(res.message, tt.want) {
				t.Errorf("message = %q, want it to contain %q", res.message, tt.want)
			}
		})
	}
}

// No accounts is a successful read with no rows: the panel must say so rather
// than claim the connection is inactive.
func TestFetchCodeBuddyUsage_NoAccounts(t *testing.T) {
	withCodeBuddyMeterServer(t, "codebuddy-cn", http.StatusOK,
		`{"code":0,"data":{"Response":{"Data":{"Accounts":[]}}}}`)

	res, ok := fetchProviderUsage(context.Background(), "codebuddy-cn", map[string]any{"accessToken": "tok"})
	if !ok {
		t.Fatal("expected fetchProviderUsage to handle codebuddy-cn")
	}
	if !strings.Contains(res.message, "codebuddy-cn") {
		t.Errorf("message = %q, want it to name codebuddy-cn", res.message)
	}
}

// A blank credential never reaches the network.
func TestFetchCodeBuddyUsage_RequiresToken(t *testing.T) {
	res := fetchCodeBuddyUsage(context.Background(), "codebuddy-cn", "", "")
	if res.message == "" {
		t.Fatal("expected a message for a blank token")
	}
	if !strings.Contains(res.message, "codebuddy-cn") {
		t.Errorf("message = %q, want it to name codebuddy-cn", res.message)
	}
}
