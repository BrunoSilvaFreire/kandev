//nolint:dupl // Native-binary ACP agents (Cursor, Goose, Junie, ...) follow the same minimal scaffold; differences are the binary name, argv, and auth surface. Shared literals live in every peer file by convention.
package agents

import (
	"context"
	_ "embed"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

//go:embed logos/junie_light.svg
var junieLogoLight []byte

//go:embed logos/junie_dark.svg
var junieLogoDark []byte

const junieBin = "junie"

var (
	_ Agent            = (*JunieACP)(nil)
	_ PassthroughAgent = (*JunieACP)(nil)
	_ InferenceAgent   = (*JunieACP)(nil)
)

// JunieACP implements Agent for JetBrains AI's Junie CLI using native ACP over
// stdin/stdout (`junie --acp=true`). Junie ships as a standalone native binary
// (the official install.sh drops it under ~/.local/bin), so the launch command
// is the bare `junie` binary discovered on PATH.
//
// Auth is the CLI's own: an interactive JetBrains AI account, the interactive
// `junie-cli` terminal login, or the JUNIE_API_KEY environment variable for
// non-interactive use. Sessions live under ~/.junie/sessions and are scoped to
// the project directory, so a workspace rebind starts a new session.
type JunieACP struct {
	StandardPassthrough
}

func NewJunieACP() *JunieACP {
	return &JunieACP{
		StandardPassthrough: StandardPassthrough{
			PermSettings: emptyPermSettings,
			Cfg: PassthroughConfig{
				Supported:      true,
				Label:          "CLI Passthrough",
				Description:    "Show terminal directly instead of chat interface",
				PassthroughCmd: NewCommand(junieBin),
				IdleTimeout:    3 * time.Second,
				BufferMaxBytes: DefaultBufferMaxBytes,
			},
		},
	}
}

func (a *JunieACP) ID() string          { return "junie-acp" }
func (a *JunieACP) Name() string        { return "Junie ACP Agent" }
func (a *JunieACP) DisplayName() string { return "Junie" }
func (a *JunieACP) Description() string {
	return "JetBrains Junie coding agent using the ACP protocol via junie --acp=true. Install the CLI with the official installer (unzip required); sign in with a JetBrains AI account or set JUNIE_API_KEY."
}
func (a *JunieACP) Enabled() bool     { return true }
func (a *JunieACP) DisplayOrder() int { return 25 }

func (a *JunieACP) Logo(v LogoVariant) []byte {
	if v == LogoDark {
		return junieLogoDark
	}
	return junieLogoLight
}

func (a *JunieACP) IsInstalled(ctx context.Context) (*DiscoveryResult, error) {
	result, err := Detect(ctx, WithCommand(junieBin))
	if err != nil {
		return result, err
	}
	result.SupportsMCP = true
	result.Capabilities = DiscoveryCapabilities{
		SupportsSessionResume: true,
	}
	return result, nil
}

func (a *JunieACP) BuildCommand(_ CommandOptions) Command {
	return Cmd(junieBin, "--acp=true").Build()
}

func (a *JunieACP) Runtime() *RuntimeConfig {
	canRecover := true
	return &RuntimeConfig{
		Cmd:             Cmd(junieBin, "--acp=true").Build(),
		WorkingDir:      "{workspace}",
		Env:             map[string]string{},
		ResourceLimits:  DefaultResourceLimits,
		Protocol:        agent.ProtocolACP,
		ProjectSkillDir: DefaultProjectSkillDir,
		UserSkillDir:    ".agents/skills",
		SessionConfig: SessionConfig{
			NativeSessionResume: true,
			// Junie scopes sessions to their project directory: session/load
			// with a changed cwd returns no session model config, so a rebind
			// must start a new session.
			NewSessionOnWorkspaceRebind: true,
			CanRecover:                  &canRecover,
			SessionDirTemplate:          "{home}/.junie",
			SessionDirTarget:            "/root/.junie",
		},
	}
}

func (a *JunieACP) RemoteAuth() *RemoteAuth {
	return &RemoteAuth{
		Methods: []RemoteAuthMethod{
			{
				Type:      "env",
				EnvVar:    "JUNIE_API_KEY",
				SetupHint: "Generate a token at https://junie.jetbrains.com/cli and export it as JUNIE_API_KEY.",
			},
		},
	}
}

func (a *JunieACP) InstallScript() string {
	// Keep the installer in a temporary file so curl failures cannot be hidden
	// by a successful bash exit status, then make sure ~/.local/bin (where the
	// official installer drops the binary) is on PATH for the rest of the
	// prepare script and for future shells.
	return `set -eu
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fsSL https://junie.jetbrains.com/install.sh -o "$tmp"
bash "$tmp"
export PATH="$HOME/.local/bin:$PATH"
command -v junie >/dev/null 2>&1`
}

func (a *JunieACP) PermissionSettings() map[string]PermissionSetting {
	return emptyPermSettings
}

func (a *JunieACP) InferenceConfig() *InferenceConfig {
	return &InferenceConfig{
		Supported: true,
		Command:   NewCommand(junieBin, "--acp=true"),
	}
}

func (a *JunieACP) BillingType() usage.BillingType { return defaultBillingType() }
