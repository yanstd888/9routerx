// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package handlerutil

import (
	"context"
	"sync"
)

// ProbeConnectionRecorder carries the id of the connection a probe actually
// used. The dashboard pins no account, so the resolver may pick any healthy
// one; without this holder a failed verdict cannot say which account was
// tested, and an operator reads "model dead" where one account was cooldown.
type ProbeConnectionRecorder struct {
	mu sync.Mutex
	id string
}

// Set records a connection id once the resolver has chosen one.
func (r *ProbeConnectionRecorder) Set(id string) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.id = id
}

// ID returns the recorded connection id, empty when no connection was chosen.
func (r *ProbeConnectionRecorder) ID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}

type probeConnectionRecorderContextKey struct{}

// WithProbeConnectionRecorder installs a recorder on the probe context.
func WithProbeConnectionRecorder(ctx context.Context, recorder *ProbeConnectionRecorder) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, probeConnectionRecorderContextKey{}, recorder)
}

// ProbeConnectionRecorderFrom returns the recorder installed on ctx, if any.
func ProbeConnectionRecorderFrom(ctx context.Context) *ProbeConnectionRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(probeConnectionRecorderContextKey{}).(*ProbeConnectionRecorder)
	return recorder
}

type probeContextKey struct{}

var probeContextKeyVal = probeContextKey{}

// WithProbeContext marks a context as belonging to a model test probe.
func WithProbeContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, probeContextKeyVal, true)
}

// IsProbeContext reports whether the context belongs to a model test probe.
func IsProbeContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(probeContextKeyVal).(bool)
	return v
}
