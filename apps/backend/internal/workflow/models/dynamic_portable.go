package models

import (
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/common/tags"
)

// DynamicAgentProfilePortable is the portable descriptor of a generated or
// user-authored Dynamic Profile. It carries only the scheduling identity and
// soft preferences that a legacy tagged step converts into; per-candidate
// provider-error policies are applied at provisioning time from the canonical
// defaults, so the portable shape stays bounded.
type DynamicAgentProfilePortable struct {
	// MigratedFrom is the stable provenance marker, for example
	// "workflow_allowed_tags:<step-id>". Empty for user-authored profiles.
	MigratedFrom string `json:"migrated_from,omitempty" yaml:"migrated_from,omitempty"`
	// PreferredTags and AvoidedTags are soft scheduling preferences.
	PreferredTags []string `json:"preferred_tags,omitempty" yaml:"preferred_tags,omitempty"`
	AvoidedTags   []string `json:"avoided_tags,omitempty" yaml:"avoided_tags,omitempty"`
	// Candidates are the ordered explicit concrete candidates.
	Candidates []DynamicAgentCandidatePortable `json:"candidates" yaml:"candidates"`
}

// DynamicAgentCandidatePortable is one ordered concrete candidate descriptor.
// CandidateProfileID is the source instance's concrete profile ID when known;
// a cross-instance import resolves the descriptor instead.
type DynamicAgentCandidatePortable struct {
	AgentProfile       AgentProfilePortable `json:"agent_profile" yaml:"agent_profile"`
	CandidateProfileID string               `json:"candidate_profile_id,omitempty" yaml:"candidate_profile_id,omitempty"`
	Enabled            bool                 `json:"enabled" yaml:"enabled"`
}

// ErrTaggedStepConversion signals that a legacy tagged step cannot be
// converted without guessing a profile.
var ErrTaggedStepConversion = errors.New("tagged step conversion failed")

// MigratedFromTaggedStepPrefix is the provenance marker written for a profile
// generated from a legacy tagged step. It is the deterministic generated
// identity: <prefix><scoped-step-id>.
const MigratedFromTaggedStepPrefix = "workflow_allowed_tags:"

// ConcreteProfileSnapshot is the converter's view of one available concrete
// profile: its identity, canonical tags, and portable descriptor.
type ConcreteProfileSnapshot struct {
	ID        string
	Tags      []string
	AgentName string
	Model     string
	Mode      string
}

// TaggedStepConversion is the pure, deterministic result of converting one
// legacy tagged step. AgentProfile is the rewritten step binding.
type TaggedStepConversion struct {
	StepID       string
	Dynamic      DynamicAgentProfilePortable
	AgentProfile AgentProfilePortable
}

// ConvertTaggedStep deterministically converts one legacy tagged step. It
// filters the snapshot to profiles matching at least one canonical allowed tag,
// orders them by profile ID, appends the distinct original fallback when it is
// present in the snapshot, copies the allowed tags into preferred_tags, leaves
// avoided_tags empty, and stamps the provenance marker. It returns
// ErrTaggedStepConversion when there is no matching candidate and no valid
// fallback.
func ConvertTaggedStep(
	stepID string,
	allowedTags []string,
	fallbackProfileID string,
	snapshot []ConcreteProfileSnapshot,
) (TaggedStepConversion, error) {
	if stepID == "" {
		return TaggedStepConversion{}, fmt.Errorf("%w: step id is required", ErrTaggedStepConversion)
	}
	canonical, err := tags.Canonical(allowedTags)
	if err != nil {
		return TaggedStepConversion{}, fmt.Errorf("%w: allowed_tags: %v", ErrTaggedStepConversion, err)
	}
	if len(canonical) == 0 {
		return TaggedStepConversion{}, fmt.Errorf("%w: step %s has no allowed_tags", ErrTaggedStepConversion, stepID)
	}

	byID := make(map[string]ConcreteProfileSnapshot, len(snapshot))
	for _, profile := range snapshot {
		if profile.ID == "" {
			continue
		}
		byID[profile.ID] = profile
	}

	matching := make([]ConcreteProfileSnapshot, 0, len(snapshot))
	for _, profile := range snapshot {
		if profile.ID == "" {
			continue
		}
		if matchesCanonicalTags(profile.Tags, canonical) {
			matching = append(matching, profile)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].ID < matching[j].ID })

	seen := make(map[string]struct{}, len(matching)+1)
	candidates := make([]DynamicAgentCandidatePortable, 0, len(matching)+1)
	for _, profile := range matching {
		if _, duplicate := seen[profile.ID]; duplicate {
			continue
		}
		seen[profile.ID] = struct{}{}
		candidates = append(candidates, dynamicCandidatePortable(profile))
	}
	if fallbackProfileID != "" {
		if _, already := seen[fallbackProfileID]; !already {
			if fallback, ok := byID[fallbackProfileID]; ok {
				seen[fallbackProfileID] = struct{}{}
				candidates = append(candidates, dynamicCandidatePortable(fallback))
			}
		}
	}
	if len(candidates) == 0 {
		return TaggedStepConversion{}, fmt.Errorf(
			"%w: step %s matched no concrete profile for tags %v and had no valid fallback",
			ErrTaggedStepConversion, stepID, canonical,
		)
	}

	dynamic := DynamicAgentProfilePortable{
		MigratedFrom:  MigratedFromTaggedStepPrefix + stepID,
		PreferredTags: canonical,
		Candidates:    candidates,
	}
	return TaggedStepConversion{
		StepID:       stepID,
		Dynamic:      dynamic,
		AgentProfile: AgentProfilePortable{Dynamic: &dynamic},
	}, nil
}

// BatchTaggedStepConversion is the pure result of converting every legacy
// tagged step in one portable export. Export is a copy; the caller's document
// is not mutated. ByStep is keyed by "<workflow-index>:<step-position>".
type BatchTaggedStepConversion struct {
	Export *WorkflowExport
	ByStep map[string]TaggedStepConversion
}

// ConvertTaggedSteps returns a copy of the export whose legacy tagged steps are
// rewritten to generated Dynamic Profile descriptors. Steps that already carry
// a Dynamic Profile descriptor, have no allowed tags, or target a session are
// left untouched, matching legacy conversion bypass rules. scope separates
// generated identities that share a workflow/step name across workspaces.
func ConvertTaggedSteps(export *WorkflowExport, snapshot []ConcreteProfileSnapshot, scope string) (BatchTaggedStepConversion, error) {
	result := BatchTaggedStepConversion{
		ByStep: make(map[string]TaggedStepConversion),
	}
	if export == nil {
		return result, nil
	}
	copied := *export
	copied.Workflows = make([]WorkflowPortable, len(export.Workflows))
	for workflowIndex, workflow := range export.Workflows {
		portableWorkflow := workflow
		portableWorkflow.Steps = make([]StepPortable, len(workflow.Steps))
		copy(portableWorkflow.Steps, workflow.Steps)
		for stepIndex := range portableWorkflow.Steps {
			step := portableWorkflow.Steps[stepIndex]
			if len(step.AllowedTags) == 0 || step.SessionTarget != nil {
				continue
			}
			if step.AgentProfile != nil && step.AgentProfile.Dynamic != nil {
				continue
			}
			fallbackProfileID := ""
			if step.AgentProfile != nil {
				fallbackProfileID = matchSnapshotByDescriptor(snapshot, *step.AgentProfile)
			}
			stepID := scopedStepID(scope, workflow.Name, step.Position)
			conversion, err := ConvertTaggedStep(stepID, step.AllowedTags, fallbackProfileID, snapshot)
			if err != nil {
				return BatchTaggedStepConversion{}, err
			}
			step.AgentProfile = cloneAgentProfilePortable(conversion.AgentProfile)
			step.AllowedTags = append([]string{}, conversion.Dynamic.PreferredTags...)
			portableWorkflow.Steps[stepIndex] = step
			result.ByStep[fmt.Sprintf("%d:%d", workflowIndex, step.Position)] = conversion
		}
		copied.Workflows[workflowIndex] = portableWorkflow
	}
	result.Export = &copied
	return result, nil
}

// ConvertTaggedWorkflows converts every non-skipped workflow in an export,
// returning a copy. Skipped workflows are passed through unconverted so a
// workflow the import will skip can never fail conversion or be provisioned.
func ConvertTaggedWorkflows(
	export *WorkflowExport,
	snapshot []ConcreteProfileSnapshot,
	scope string,
	skipNames map[string]bool,
) (*WorkflowExport, error) {
	if export == nil {
		return nil, nil
	}
	converted := *export
	converted.Workflows = make([]WorkflowPortable, len(export.Workflows))
	for workflowIndex, workflow := range export.Workflows {
		if skipNames[workflow.Name] {
			converted.Workflows[workflowIndex] = workflow
			continue
		}
		sub := &WorkflowExport{Version: export.Version, Type: export.Type, Workflows: []WorkflowPortable{workflow}}
		conversion, err := ConvertTaggedSteps(sub, snapshot, scope)
		if err != nil {
			return nil, err
		}
		converted.Workflows[workflowIndex] = conversion.Export.Workflows[0]
	}
	return &converted, nil
}

// scopedStepID derives the deterministic per-step identity used for generated
// profile markers. The scope keeps two workspaces' identically named workflows
// from sharing one generated profile.
func scopedStepID(scope, workflowName string, position int) string {
	base := fmt.Sprintf("workflow:%s:step:%d", workflowName, position)
	if scope == "" {
		return base
	}
	return scope + ":" + base
}

func cloneAgentProfilePortable(profile AgentProfilePortable) *AgentProfilePortable {
	cloned := profile
	if profile.Dynamic != nil {
		dynamic := *profile.Dynamic
		dynamic.PreferredTags = append([]string{}, profile.Dynamic.PreferredTags...)
		dynamic.AvoidedTags = append([]string{}, profile.Dynamic.AvoidedTags...)
		dynamic.Candidates = append([]DynamicAgentCandidatePortable{}, profile.Dynamic.Candidates...)
		cloned.Dynamic = &dynamic
	}
	return &cloned
}

// matchSnapshotByDescriptor resolves a portable fallback descriptor to a local
// profile. When several profiles share the triple it returns the smallest ID so
// the result does not depend on snapshot order.
func matchSnapshotByDescriptor(snapshot []ConcreteProfileSnapshot, profile AgentProfilePortable) string {
	best := ""
	for _, candidate := range snapshot {
		if candidate.AgentName != profile.AgentName || candidate.Model != profile.Model || candidate.Mode != profile.Mode {
			continue
		}
		if best == "" || candidate.ID < best {
			best = candidate.ID
		}
	}
	return best
}

func dynamicCandidatePortable(profile ConcreteProfileSnapshot) DynamicAgentCandidatePortable {
	return DynamicAgentCandidatePortable{
		AgentProfile: AgentProfilePortable{
			AgentName: profile.AgentName,
			Model:     profile.Model,
			Mode:      profile.Mode,
		},
		CandidateProfileID: profile.ID,
		Enabled:            true,
	}
}

func matchesCanonicalTags(profileTags, canonicalAllowed []string) bool {
	if len(canonicalAllowed) == 0 {
		return false
	}
	allowed := make(map[string]struct{}, len(canonicalAllowed))
	for _, tag := range canonicalAllowed {
		allowed[tag] = struct{}{}
	}
	for _, tag := range profileTags {
		if _, ok := allowed[tag]; ok {
			return true
		}
	}
	return false
}
