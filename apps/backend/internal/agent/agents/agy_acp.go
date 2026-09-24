package agents

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

const (
	agyBin    = "agy"
	agyACPPkg = "agy-acp"
)

var (
	_ Agent                  = (*AgyACP)(nil)
	_ PassthroughAgent       = (*AgyACP)(nil)
	_ InferenceAgent         = (*AgyACP)(nil)
	_ LoginAgent             = (*AgyACP)(nil)
	_ ManagedNPMRuntimeAgent = (*AgyACP)(nil)
	_ NativeBinaryAgent      = (*AgyACP)(nil)
)

// AgyACP bridges the locally authenticated agy CLI through the third-party
// agy-acp adapter. It is deliberately distinct from Google's antigravity-acp
// kernel, including its credentials and persisted sessions.
type AgyACP struct{ StandardPassthrough }

func NewAgyACP() *AgyACP {
	return &AgyACP{StandardPassthrough: StandardPassthrough{
		PermSettings: emptyPermSettings,
		Cfg: PassthroughConfig{
			Supported:      true,
			Label:          "CLI Passthrough",
			Description:    "Show terminal directly instead of chat interface",
			PassthroughCmd: NewCommand(agyBin),
			ModelFlag:      NewParam("--model", "{model}"),
			IdleTimeout:    3 * time.Second,
			BufferMaxBytes: DefaultBufferMaxBytes,
			MCPStrategy:    mcpconfig.AntigravityStrategy{},
		},
	}}
}

func (a *AgyACP) ID() string          { return agyACPPkg }
func (a *AgyACP) Name() string        { return "Antigravity CLI ACP Agent" }
func (a *AgyACP) DisplayName() string { return "Antigravity CLI" }
func (a *AgyACP) Description() string {
	return "Logged-in native agy CLI through the third-party agy-acp adapter. Third-party Antigravity access can risk account suspension."
}
func (a *AgyACP) Enabled() bool     { return true }
func (a *AgyACP) DisplayOrder() int { return 24 }
func (a *AgyACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return antigravityACPLogoDark
	}
	return antigravityACPLogoLight
}

func (a *AgyACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommandCheck(agyBin, "--version"))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{SupportsSessionResume: true}
	return result, nil
}

func (a *AgyACP) NativeBinaryName() string { return agyACPPkg }

func (a *AgyACP) BuildCommand(opts CommandOptions) Command {
	if opts.PreferNativeBinary {
		return a.ManagedNPMRuntime().NativeCommand()
	}
	return a.ManagedNPMRuntime().ACPCommand(opts.ManagedRuntimeVersion)
}

func (a *AgyACP) ManagedNPMRuntime() ManagedNPMRuntimeSpec {
	return ManagedNPMRuntimeSpec{
		Package:        agyACPPkg,
		DefaultVersion: MustDefaultManagedNPMRuntimeVersion(agyACPPkg),
		ACPArgs:        []string{"--no-sandbox", "--dangerously-skip-permissions"},
		NativeBinary:   agyACPPkg,
	}
}

func (a *AgyACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:                a.ManagedNPMRuntime().CachedACPCommand(),
		WorkingDir:         "{workspace}",
		Env:                map[string]string{},
		ResourceLimits:     DefaultResourceLimits,
		Protocol:           agent.ProtocolACP,
		ProjectSkillDir:    DefaultProjectSkillDir,
		ProjectMCPStrategy: mcpconfig.AntigravityStrategy{},
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			CanRecover:          &canRecover,
			SessionDirTemplate:  "{home}",
			SessionDirTarget:    "/root",
		},
	}
}
func (a *AgyACP) RemoteAuth() *RemoteAuth { return nil }
func (a *AgyACP) LoginCommand() *LoginCommand {
	return &LoginCommand{Cmd: []string{agyBin}, Description: "Complete Antigravity sign-in, then quit."}
}
func (a *AgyACP) InstallScript() string                            { return "" }
func (a *AgyACP) PermissionSettings() map[string]PermissionSetting { return emptyPermSettings }
func (a *AgyACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{Supported: true, Command: a.ManagedNPMRuntime().CachedACPCommand()}
}
func (a *AgyACP) BillingType() usage.BillingType { return usage.BillingTypeSubscription }
