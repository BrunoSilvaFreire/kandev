package agents

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
)

func TestAgyACP(t *testing.T) {
	a := NewAgyACP()
	if a.ID() != "agy-acp" || a.DisplayName() != "Antigravity CLI" {
		t.Fatalf("identity = %q / %q", a.ID(), a.DisplayName())
	}
	if a.RemoteAuth() != nil || a.BillingType() != usage.BillingTypeSubscription || a.InstallScript() != "" {
		t.Fatal("unexpected auth, billing, or installation contract")
	}
	if len(a.Logo(LogoLight)) == 0 || len(a.Logo(LogoDark)) == 0 {
		t.Fatal("logos must reuse Antigravity assets")
	}
	if got := a.BuildCommand(CommandOptions{}).Args(); len(got) != 6 || got[3] != "agy-acp@0.5.2" || got[4] != "--no-sandbox" || got[5] != "--dangerously-skip-permissions" {
		t.Fatalf("command = %v", got)
	}
	if a.NativeBinaryName() != "agy-acp" {
		t.Fatalf("NativeBinaryName = %q, want %q", a.NativeBinaryName(), "agy-acp")
	}
	if got := a.BuildCommand(CommandOptions{PreferNativeBinary: true}).Args(); len(got) != 3 || got[0] != "agy-acp" || got[1] != "--no-sandbox" || got[2] != "--dangerously-skip-permissions" {
		t.Fatalf("native command = %v", got)
	}
	if got := a.PassthroughConfig().PassthroughCmd.Args(); len(got) != 1 || got[0] != "agy" {
		t.Fatalf("passthrough = %v", got)
	}
	if _, ok := a.PassthroughConfig().MCPStrategy.(mcpconfig.AntigravityStrategy); !ok {
		t.Fatal("passthrough MCP strategy")
	}
	rt := a.Runtime()
	if _, ok := rt.ProjectMCPStrategy.(mcpconfig.AntigravityStrategy); !ok || !rt.SessionConfig.NativeSessionResume || rt.SessionConfig.SessionDirTemplate != "{home}" {
		t.Fatal("runtime contract")
	}
	if a.LoginCommand().Cmd[0] != "agy" {
		t.Fatal("login must use agy")
	}
}

func TestAgyACP_DiscoveryRequiresAgyVersion(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result, err := NewAgyACP().IsInstalled(context.Background())
	if err != nil || result.Available {
		t.Fatalf("IsInstalled = %+v, %v", result, err)
	}
}
