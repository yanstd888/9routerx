package daemon

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/proc"
)

func TestStartUnhealthyChildIsKilled(t *testing.T) {
	dir := isolate(t)
	addr := unusedStartAddress(t)
	t.Setenv("9ROUTERX_START_HELPER", "unhealthy")
	t.Setenv("9ROUTERX_START_HELPER_ADDR", addr)
	rescueStartChild(t, dir)

	started := time.Now()
	res, err := Start("http://"+addr, []string{"-test.run=^TestDaemonStartHelper$"})
	if err == nil || !strings.Contains(err.Error(), "no response from") {
		t.Fatalf("Start() error = %v, want the unchanged health timeout", err)
	}
	if elapsed := time.Since(started); elapsed < StartupTimeout {
		t.Errorf("Start() returned after %s, before StartupTimeout %s", elapsed, StartupTimeout)
	}
	pid := capturedStartPID(t, dir)
	if !strings.HasPrefix(err.Error(), "daemon.Start: failed startup daemon stopped (pid "+strconv.Itoa(pid)+"): ") {
		t.Errorf("Start() must report the failed daemon stopped: %v", err)
	}
	if res.PID != pid || res.Existing {
		t.Errorf("Start() = %+v, want newly spawned pid %d", res, pid)
	}
	alive := proc.Alive(pid)
	t.Logf("Start returned health error; captured pid=%d alive=%t", pid, alive)
	if alive {
		t.Errorf("Start() left its unhealthy child alive: pid %d", pid)
	}
	if _, err := os.Stat(PIDFile()); !os.IsNotExist(err) {
		t.Errorf("failed Start must remove its dead child's claim, stat error = %v", err)
	}
}

func TestStartHealthyChildKeepsProcessAndClaim(t *testing.T) {
	dir := isolate(t)
	addr := unusedStartAddress(t)
	t.Setenv("9ROUTERX_START_HELPER", "healthy")
	t.Setenv("9ROUTERX_START_HELPER_ADDR", addr)
	rescueStartChild(t, dir)

	res, err := Start("http://"+addr, []string{"-test.run=^TestDaemonStartHelper$"})
	if err != nil {
		t.Fatalf("Start() = %v, helper log: %s", err, LogTail(20))
	}
	pid := capturedStartPID(t, dir)
	if res.PID != pid || res.Existing || res.URL != "http://"+addr {
		t.Errorf("Start() = %+v, want newly spawned pid %d at %s", res, pid, addr)
	}
	if !proc.Alive(pid) {
		t.Errorf("successful Start killed its healthy child: pid %d", pid)
	}
	if got := readClaim(); got != (claim{pid: pid, exePath: selfExecutable(t)}) {
		t.Errorf("successful Start claim = %+v, want healthy child's claim", got)
	}
	if !probeHealthy("http://" + addr + HealthPath) {
		t.Error("healthy child stopped answering after Start returned")
	}
}

func TestCleanupFailedStartProcessAndErrors(t *testing.T) {
	startErr := errors.New("health failed")
	killErr := errors.New("kill refused")
	tests := []struct {
		name      string
		kill      bool
		killErr   error
		wantAlive bool
		wantClaim bool
	}{
		{name: "kill failure keeps live claim and both errors", killErr: killErr, wantAlive: true, wantClaim: true},
		{name: "accepted kill without exit keeps live claim", wantAlive: true, wantClaim: true},
		{name: "confirmed exit removes matching claim", kill: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			pid := livePID(t)
			unrelated := livePID(t)
			exe := selfExecutable(t)
			raw := pidLine(pid, exe)
			writePIDFile(t, raw)
			calls := 0
			started := time.Now()
			err := cleanupFailedStart(pid, exe, startErr, func(got int) error {
				calls++
				if got != pid {
					t.Fatalf("cleanup attempted to kill pid %d, want spawned pid %d", got, pid)
				}
				if c := readClaim(); c != (claim{pid: pid, exePath: exe}) {
					t.Errorf("claim cleared before kill: %+v", c)
				}
				if tt.kill {
					return proc.ForceKill(got)
				}
				return tt.killErr
			})
			elapsed := time.Since(started)
			if calls != 1 {
				t.Errorf("kill calls = %d, want 1", calls)
			}
			if !errors.Is(err, startErr) {
				t.Errorf("cleanup lost startup error: %v", err)
			}
			if tt.killErr != nil && !errors.Is(err, tt.killErr) {
				t.Errorf("cleanup lost kill error: %v", err)
			}
			if tt.wantAlive && tt.killErr == nil {
				if err == nil || !errors.Is(err, startErr) || !strings.Contains(err.Error(), "still alive") {
					t.Errorf("cleanup hid failed exit confirmation: %v", err)
				}
				if elapsed < 2*time.Second || elapsed > 7*time.Second {
					t.Errorf("exit confirmation took %s, want a bounded two-second wait", elapsed)
				}
			}
			if tt.wantAlive && err != nil && strings.Contains(err.Error(), "daemon stopped") {
				t.Errorf("unconfirmed cleanup must not report daemon stopped: %v", err)
			}
			if !tt.wantAlive {
				assertFailedStartStopped(t, pid, err, startErr)
			}
			if alive := proc.Alive(pid); alive != tt.wantAlive {
				t.Errorf("spawned pid %d alive = %t, want %t", pid, alive, tt.wantAlive)
			}
			if !proc.Alive(unrelated) {
				t.Error("startup cleanup killed an unrelated helper")
			}
			data, readErr := os.ReadFile(PIDFile())
			if tt.wantClaim {
				if readErr != nil || string(data) != raw {
					t.Errorf("unconfirmed exit must keep matching claim: data=%q error=%v", data, readErr)
				}
			} else if !os.IsNotExist(readErr) {
				t.Errorf("confirmed exit must remove matching claim: %v", readErr)
			}
		})
	}
}

func TestCleanupFailedStartKeepsReplacementClaim(t *testing.T) {
	for _, samePID := range []bool{false, true} {
		name := "replacement pid"
		if samePID {
			name = "replacement executable"
		}
		t.Run(name, func(t *testing.T) {
			isolate(t)
			pid := livePID(t)
			replacementPID := livePID(t)
			exe := selfExecutable(t)
			writePIDFile(t, pidLine(pid, exe))
			replacement := pidLine(replacementPID, exe)
			if samePID {
				replacement = pidLine(pid, foreignExecutable(t))
			}
			startErr := errors.New("health failed")
			err := cleanupFailedStart(pid, exe, startErr, func(got int) error {
				if got != pid {
					t.Fatalf("kill targeted replacement: pid %d, want spawned pid %d", got, pid)
				}
				writePIDFile(t, replacement)
				return proc.ForceKill(got)
			})
			assertFailedStartStopped(t, pid, err, startErr)
			if proc.Alive(pid) {
				t.Error("failed child survived cleanup")
			}
			if !proc.Alive(replacementPID) {
				t.Error("startup cleanup killed the replacement process")
			}
			if data, err := os.ReadFile(PIDFile()); err != nil || string(data) != replacement {
				t.Errorf("replacement claim changed: data=%q error=%v", data, err)
			}
		})
	}
}

func TestCleanupFailedStartAlreadyExitedDoesNotKill(t *testing.T) {
	isolate(t)
	pid := livePID(t)
	exe := selfExecutable(t)
	writePIDFile(t, pidLine(pid, exe))
	if err := proc.ForceKill(pid); err != nil {
		t.Fatalf("stop helper before cleanup: %v", err)
	}
	if !waitExit(pid, 2000) {
		t.Fatal("helper did not exit before cleanup")
	}
	startErr := errors.New("child exited before ready")
	err := cleanupFailedStart(pid, exe, startErr, func(int) error {
		t.Fatal("cleanup must not kill an already exited child")
		return nil
	})
	assertFailedStartStopped(t, pid, err, startErr)
	if _, err := os.Stat(PIDFile()); !os.IsNotExist(err) {
		t.Errorf("dead child's matching claim remains: %v", err)
	}
}

func TestCleanupFailedStartMissingClaimStillKillsOwnedChild(t *testing.T) {
	isolate(t)
	pid := livePID(t)
	startErr := errors.New("health failed")
	err := cleanupFailedStart(pid, selfExecutable(t), startErr, proc.ForceKill)
	assertFailedStartStopped(t, pid, err, startErr)
	if proc.Alive(pid) {
		t.Error("missing claim prevented cleanup of the owned spawn")
	}
	if _, err := os.Stat(PIDFile()); !os.IsNotExist(err) {
		t.Errorf("cleanup created a claim: %v", err)
	}
}

func assertFailedStartStopped(t *testing.T, pid int, err, startErr error) {
	t.Helper()
	if !errors.Is(err, startErr) {
		t.Errorf("successful cleanup lost the startup error: %v", err)
	}
	prefix := "daemon.Start: failed startup daemon stopped (pid " + strconv.Itoa(pid) + "): "
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("successful cleanup must report its daemon stopped with pid %d: %v", pid, err)
	}
}

func TestDaemonStartHelper(t *testing.T) {
	mode := os.Getenv("9ROUTERX_START_HELPER")
	if mode == "" {
		t.Skip("spawned Start helper only")
	}
	if !IsBackgroundProcess() {
		t.Fatal("Start helper was not spawned as a background process")
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DATA_DIR"), "spawned.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatalf("record spawned pid: %v", err)
	}
	if mode == "unhealthy" {
		time.Sleep(2 * time.Minute)
		return
	}
	if mode != "healthy" {
		t.Fatalf("unknown Start helper mode %q", mode)
	}
	listener, err := net.Listen("tcp", os.Getenv("9ROUTERX_START_HELPER_ADDR"))
	if err != nil {
		t.Fatalf("listen for helper health: %v", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != HealthPath {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
		ReadHeaderTimeout: time.Second,
	}
	time.AfterFunc(2*time.Minute, func() { _ = server.Close() })
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve helper health: %v", err)
	}
}

func unusedStartAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("choose unused loopback port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port before Start: %v", err)
	}
	return addr
}

func capturedStartPID(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "spawned.pid"))
	if err != nil {
		t.Fatalf("read helper's captured pid: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid <= 0 {
		t.Fatalf("captured pid = %q, parse error = %v", data, err)
	}
	return pid
}

func rescueStartChild(t *testing.T, dir string) {
	t.Helper()
	// Read the child's own record even if Start fails before returning its PID.
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(dir, "spawned.pid"))
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Errorf("read child record for rescue: %v", err)
			return
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil || pid <= 0 {
			t.Errorf("invalid rescue pid %q: %v", data, err)
			return
		}
		if proc.Alive(pid) {
			if err := proc.ForceKill(pid); err != nil {
				t.Errorf("rescue kill pid %d: %v", pid, err)
			}
		}
		dead := waitExit(pid, 2000)
		t.Logf("rescue cleanup captured pid=%d exited=%t", pid, dead)
		if !dead {
			t.Errorf("rescue left helper pid %d alive", pid)
		}
	})
}
