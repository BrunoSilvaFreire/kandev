package agents

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestOpenCodeACPUsesManagedRuntime(t *testing.T) {
	// Pin PATH to an empty dir so the executor-safe configuration is
	// deterministic.
	t.Setenv("PATH", t.TempDir())
	a := NewOpenCodeACP()
	want := a.ManagedNPMRuntime().CachedACPCommand().Args()

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	if got, wantInstall := a.InstallScript(), "npm install -g opencode-ai"; got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

// TestOpenCodeACPBuildCommandPrefersNativeBinary is the CodeNomad-style
// binary-first launch: when the lifecycle probe finds `opencode` on PATH,
// BuildCommand emits the direct binary instead of the per-launch npx
// resolution. It is the regression test for "ACP initialize failed: peer
// disconnected before response", which happened because npx exited with
// empty stdout on a stale packument cache / missing postinstall bootstrap.
func TestOpenCodeACPBuildCommandPrefersNativeBinary(t *testing.T) {
	a := NewOpenCodeACP()

	if name := a.NativeBinaryName(); name != "opencode" {
		t.Fatalf("NativeBinaryName() = %q, want %q", name, "opencode")
	}

	wantNative := []string{"opencode", "acp", "--print-logs"}
	if got := a.BuildCommand(CommandOptions{PreferNativeBinary: true}).Args(); !slices.Equal(got, wantNative) {
		t.Fatalf("BuildCommand(PreferNativeBinary) = %#v, want %#v", got, wantNative)
	}

	// Default (probe absent / containers / remotes) keeps the managed npx runtime.
	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, a.ManagedNPMRuntime().CachedACPCommand().Args()) {
		t.Fatalf("BuildCommand(default) = %#v, want npx fallback", got)
	}
}

// TestOpenCodeACPInferenceConfigPrefersNativeBinaryOnPath pins the host-utility
// bootstrap to the standalone binary when it is installed, so the ACP probe
// never depends on npm cache state.
func TestOpenCodeACPInferenceConfigPrefersNativeBinaryOnPath(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 0")

	a := NewOpenCodeACP()
	want := []string{"opencode", "acp", "--print-logs"}
	if got := a.HostUtilityInferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("HostUtilityInferenceConfig().Command = %#v, want %#v", got, want)
	}
}

func TestOpenCodeACPInferenceConfigStaysExecutorSafeOnHostPath(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 0")

	a := NewOpenCodeACP()
	want := a.ManagedNPMRuntime().CachedACPCommand().Args()
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("InferenceConfig().Command = %#v, want executor-safe npx command %#v", got, want)
	}
}

// TestOpenCodeACPOmitsCaseSensitiveLogLevel guards the OpenCode 2.0 startup
// regression: 1.x and 2.x accept opposite-case level names for `--log-level`,
// and a wrong-case value makes `opencode acp` exit before the ACP handshake
// (surfacing as "ACP initialize handshake failed: ... file already closed").
// The fix is to pass no `--log-level` at all, which the version-unknown native
// binary path cannot resolve either. Any reintroduction fails here.
func TestOpenCodeACPOmitsCaseSensitiveLogLevel(t *testing.T) {
	a := NewOpenCodeACP()

	managed := a.ManagedNPMRuntime().ACPArgs
	if !slices.Equal(managed, []string{"acp", "--print-logs"}) {
		t.Fatalf("managed ACP args = %#v, want --print-logs only", managed)
	}
	for _, args := range [][]string{
		a.BuildCommand(CommandOptions{}).Args(),
		a.BuildCommand(CommandOptions{PreferNativeBinary: true}).Args(),
		a.Runtime().Cmd.Args(),
	} {
		if slices.Contains(args, "--log-level") {
			t.Errorf("opencode ACP args %#v must not pin a case-sensitive --log-level", args)
		}
		if !slices.Contains(args, "--print-logs") {
			t.Errorf("opencode ACP args %#v lost --print-logs", args)
		}
	}
}

func TestOpenCodeACPDiscoveryRecognizesAuthenticationHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture is not executable on Windows")
	}
	binaryPath := writeOpenCodeTestBinary(t, "exit 7")

	a := NewOpenCodeACP()
	result, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
	if result.MatchedPath != binaryPath {
		t.Fatalf("IsInstalled() MatchedPath = %q, want %q", result.MatchedPath, binaryPath)
	}
}

func TestOpenCodeACPDiscoveryDoesNotRunVersionCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture is not executable on Windows")
	}
	writeOpenCodeTestBinary(t, "exit 7")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
}

func writeOpenCodeTestBinary(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, agentTestExecutableName("opencode"))
	contents := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(binaryPath, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}
	t.Setenv("PATH", dir)
	return binaryPath
}

func agentTestExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// TestOpenCodeACPRuntime_RequiresProcessKill is the regression test for GH
// issue #1247: opencode acp keeps its HTTP server + MCP child tree alive
// when stdin closes, so its RuntimeConfig must signal that the process
// group should be reaped immediately. Without this flag the ACP adapter
// returns RequiresProcessKill=false and the process manager waits for the
// graceful EOF path before it falls back to process-group cleanup.
func TestOpenCodeACPRuntime_RequiresProcessKill(t *testing.T) {
	rt := NewOpenCodeACP().Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if !rt.RequiresProcessKill {
		t.Error("RequiresProcessKill = false; opencode acp must opt into process-group kill")
	}
}

// TestACPAgents_DefaultProcessKill confirms the rest of the ACP agents
// stick with the default (false). They communicate over plain stdin/stdout
// and should get a short graceful EOF path before the process manager reaps
// any remaining process-group descendants.
func TestACPAgents_DefaultProcessKill(t *testing.T) {
	cases := []struct {
		name  string
		agent Agent
	}{
		{"claude", NewClaudeACP()},
		{"codex", NewCodexACP()},
		{"cursor", NewCursorACP()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.agent.Runtime()
			if rt == nil {
				t.Fatalf("%s Runtime() returned nil", tc.name)
			}
			if rt.RequiresProcessKill {
				t.Errorf("%s RequiresProcessKill = true; expected default false", tc.name)
			}
		})
	}
}

func TestOpenCodeACPRemoteAuth(t *testing.T) {
	auth := NewOpenCodeACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected files-based auth method")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "files" {
		t.Errorf("Type = %q, want %q", m.Type, "files")
	}
	if m.TargetRelDir != ".local/share/opencode" {
		t.Errorf("TargetRelDir = %q, want %q", m.TargetRelDir, ".local/share/opencode")
	}
	if m.FileConflictPolicy != RemoteAuthFileConflictPolicyMergeJSONObject {
		t.Errorf("FileConflictPolicy = %q, want %q", m.FileConflictPolicy, RemoteAuthFileConflictPolicyMergeJSONObject)
	}
	want := []string{".local/share/opencode/auth.json"}
	for _, os := range []string{"darwin", "linux"} {
		got := m.SourceFiles[os]
		if !slices.Equal(got, want) {
			t.Errorf("SourceFiles[%q] = %v, want %v", os, got, want)
		}
	}
}
