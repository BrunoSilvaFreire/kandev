package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/usage"
)

func TestJunieACP_IDAndDisplay(t *testing.T) {
	a := NewJunieACP()
	if got := a.ID(); got != "junie-acp" {
		t.Errorf("ID() = %q, want junie-acp", got)
	}
	if got := a.Name(); got != "Junie ACP Agent" {
		t.Errorf("Name() = %q, want Junie ACP Agent", got)
	}
	if got := a.DisplayName(); got != "Junie" {
		t.Errorf("DisplayName() = %q, want Junie", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 25 {
		t.Errorf("DisplayOrder() = %d, want 25", got)
	}
	if got := a.Description(); !strings.Contains(got, "junie --acp=true") {
		t.Errorf("Description should mention the ACP launch command, got %q", got)
	}
	if got := a.Description(); !strings.Contains(got, "unzip") {
		t.Errorf("Description should name the unzip prerequisite, got %q", got)
	}
}

func TestJunieACP_SessionConfig(t *testing.T) {
	rt := NewJunieACP().Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	if len(rt.Env) != 0 {
		t.Errorf("Runtime Env = %#v, want empty", rt.Env)
	}
	if len(rt.StripEnv) != 0 {
		t.Errorf("Runtime StripEnv = %#v, want empty", rt.StripEnv)
	}
	if rt.ProjectMCPStrategy != nil {
		t.Error("ProjectMCPStrategy must be nil; MCP goes through session/new")
	}

	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true (session/load in unchanged cwd)")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if !sc.NewSessionOnWorkspaceRebind {
		t.Error("NewSessionOnWorkspaceRebind = false, want true (session state is project-scoped)")
	}
	if sc.SessionDirTemplate != "{home}/.junie" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.junie", sc.SessionDirTemplate)
	}
	if sc.SessionDirTarget != "/root/.junie" {
		t.Errorf("SessionDirTarget = %q, want /root/.junie", sc.SessionDirTarget)
	}
	if rt.ProjectSkillDir != DefaultProjectSkillDir {
		t.Errorf("ProjectSkillDir = %q, want %q", rt.ProjectSkillDir, DefaultProjectSkillDir)
	}
	if rt.UserSkillDir != ".agents/skills" {
		t.Errorf("UserSkillDir = %q, want .agents/skills", rt.UserSkillDir)
	}
}

func TestJunieACP_RemoteAuthUsesAPIKeyEnv(t *testing.T) {
	auth := NewJunieACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "env" {
		t.Errorf("Type = %q, want env", m.Type)
	}
	if m.EnvVar != "JUNIE_API_KEY" {
		t.Errorf("EnvVar = %q, want JUNIE_API_KEY", m.EnvVar)
	}
	if !strings.Contains(m.SetupHint, "junie.jetbrains.com") {
		t.Errorf("SetupHint should point at the Junie CLI token page, got %q", m.SetupHint)
	}
}

func TestJunieACP_InstallScript(t *testing.T) {
	got := NewJunieACP().InstallScript()
	for _, needle := range []string{
		"set -eu",
		`tmp="$(mktemp)"`,
		`trap 'rm -f "$tmp"' EXIT`,
		"https://junie.jetbrains.com/install.sh",
		`-o "$tmp"`,
		`export PATH="$HOME/.local/bin:$PATH"`,
		"command -v junie",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("InstallScript missing %q: %q", needle, got)
		}
	}
	if strings.HasPrefix(got, "npm install -g ") {
		t.Errorf("InstallScript should use the official native installer, got npm script: %q", got)
	}
}

func TestJunieACPInstallScriptPropagatesDownloadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}

	binDir := t.TempDir()
	fakeCurl := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	cmd := exec.Command("sh", "-c", NewJunieACP().InstallScript())
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + binDir + ":/usr/bin:/bin",
	}
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("InstallScript succeeded after curl failure; output: %s", out)
	}
}

func TestJunieACPInstallScriptRunsOfficialInstaller(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}

	home := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	installer := filepath.Join(t.TempDir(), "installer.sh")
	if err := os.WriteFile(installer, []byte("#!/bin/sh\nprintf 'installer ran\\n' > \"$HOME/ran\"\n"), 0o644); err != nil {
		t.Fatalf("write installer fixture: %v", err)
	}

	// Fake curl copies the fixture installer to the -o target and lets the
	// script run it. Fake the junie binary so the post-install check passes.
	fakeCurl := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) shift; out="$1" ;;
  esac
  shift
done
cat "$FAKE_INSTALLER" > "$out"
`
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}
	fakeHomeLocalBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(fakeHomeLocalBin, 0o755); err != nil {
		t.Fatalf("mkdir fake local bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fakeHomeLocalBin, "junie"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake junie: %v", err)
	}

	script := "export FAKE_INSTALLER=" + installer + "\n" + NewJunieACP().InstallScript()
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + binDir + ":/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("InstallScript failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "ran")); err != nil {
		t.Fatalf("official installer was not executed: %v", err)
	}
}

func TestJunieACP_LogosNonEmpty(t *testing.T) {
	a := NewJunieACP()
	if len(a.Logo(LogoLight)) == 0 {
		t.Error("Logo(LogoLight) is empty")
	}
	if len(a.Logo(LogoDark)) == 0 {
		t.Error("Logo(LogoDark) is empty")
	}
	if !strings.Contains(string(a.Logo(LogoLight)), "<svg") {
		t.Error("Logo(LogoLight) is not SVG")
	}
	// Unknown variants resolve to the light logo.
	if !slices.Equal(a.Logo(LogoVariant(99)), a.Logo(LogoLight)) {
		t.Error("Logo(unknown) should return the light logo")
	}
}

func TestJunieACP_DetectionRequiresGlobalBinary(t *testing.T) {
	if _, err := exec.LookPath("junie"); err == nil {
		t.Skip("detection binary \"junie\" is on PATH; can't verify availability requirement")
	}
	result, err := NewJunieACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without junie on PATH; discovery must not imply install")
	}
}

func TestJunieACP_DetectionAcceptsJunieBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture is Unix-specific")
	}

	binDir := t.TempDir()
	juniePath := filepath.Join(binDir, junieBin)
	if err := os.WriteFile(juniePath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake junie: %v", err)
	}
	t.Setenv("PATH", binDir)

	result, err := NewJunieACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if !result.Available {
		t.Fatal("Available=false for a junie binary on PATH")
	}
	if result.MatchedPath != juniePath {
		t.Errorf("MatchedPath = %q, want %q", result.MatchedPath, juniePath)
	}
	if !result.SupportsMCP {
		t.Error("SupportsMCP = false, want true (session/new mcpServers)")
	}
	if !result.Capabilities.SupportsSessionResume {
		t.Error("SupportsSessionResume = false, want true (session/load)")
	}
}

func TestJunieACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewJunieACP()
	if len(a.PermissionSettings()) != 0 {
		t.Errorf("PermissionSettings() = %#v, want empty (agentctl auto-approve is authoritative)", a.PermissionSettings())
	}
	if got := a.BillingType(); got != usage.BillingTypeAPIKey {
		t.Errorf("BillingType() = %q, want %q", got, usage.BillingTypeAPIKey)
	}
	catalog := CatalogPermissionSettings(a)
	auto, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if auto.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Errorf("auto_approve ApplyMethod = %q, want agentctl auto-approve", auto.ApplyMethod)
	}
}
