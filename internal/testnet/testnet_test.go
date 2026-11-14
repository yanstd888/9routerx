// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package testnet

import (
	"net"
	"net/http"
	"strings"
	"testing"
)

// TestDialerAllowsLoopbackOnly pins the seal's two halves: a loopback dial
// (what every httptest server is) proceeds, and an outbound dial is refused
// with the stub-your-upstream message.
func TestDialerAllowsLoopbackOnly(t *testing.T) {
	ts := &http.Server{Handler: http.NewServeMux()}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() { _ = ts.Serve(ln) }()

	if _, err := dialer.Dial("tcp", ln.Addr().String()); err != nil {
		t.Errorf("loopback dial blocked: %v", err)
	}

	_, err = dialer.Dial("tcp", "api.elevenlabs.io:443")
	if err == nil {
		t.Fatal("outbound dial allowed through the seal")
	}
	if !strings.Contains(err.Error(), "stub your upstream") {
		t.Errorf("dial error %q does not tell the test to stub its upstream", err)
	}
}

// TestSealIdempotent proves the explicit TestMain call and the env-gated init
// can both run without the second Seal() panicking on an already-replaced
// dialer.
func TestSealIdempotent(t *testing.T) {
	Seal()
	Seal()
	if http.DefaultTransport.(*http.Transport).DialContext == nil {
		t.Error("seal left the default transport without a dialer")
	}
}
