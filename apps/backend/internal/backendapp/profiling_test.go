package backendapp

import (
	"net/http"
	"testing"
)

func TestIsProfilingEnabled(t *testing.T) {
	t.Setenv("KANDEV_PROFILE", "")
	if isProfilingEnabled() {
		t.Errorf("expected isProfilingEnabled() to be false when unset")
	}

	t.Setenv("KANDEV_PROFILE", "0")
	if isProfilingEnabled() {
		t.Errorf("expected isProfilingEnabled() to be false for '0'")
	}

	t.Setenv("KANDEV_PROFILE", "1")
	if !isProfilingEnabled() {
		t.Errorf("expected isProfilingEnabled() to be true for '1'")
	}

	t.Setenv("KANDEV_PROFILE", "true")
	if !isProfilingEnabled() {
		t.Errorf("expected isProfilingEnabled() to be true for 'true'")
	}
}

func TestStartProfilingServer(t *testing.T) {
	cleanup, endpoint, err := startProfilingServer(nil)
	if err != nil {
		t.Fatalf("startProfilingServer failed: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	}()

	resp, err := http.Get(endpoint + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET /debug/pprof/ failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /debug/pprof/ status = %d, want 200", resp.StatusCode)
	}
}
