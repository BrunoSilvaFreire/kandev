package models

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func snapshot(id string, tags ...string) ConcreteProfileSnapshot {
	return ConcreteProfileSnapshot{
		ID: id, Tags: tags, AgentName: "agent-" + id, Model: "m", Mode: "mode",
	}
}

func candidateIDs(conversion TaggedStepConversion) []string {
	ids := make([]string, 0, len(conversion.Dynamic.Candidates))
	for _, candidate := range conversion.Dynamic.Candidates {
		ids = append(ids, candidate.AgentProfile.AgentName)
	}
	return ids
}

func TestConvertTaggedStepStableRegardlessOfSourceOrder(t *testing.T) {
	profiles := []ConcreteProfileSnapshot{
		snapshot("b", "claude"),
		snapshot("a", "claude"),
		snapshot("c", "codex"),
	}
	first, err := ConvertTaggedStep("step-1", []string{"claude"}, "", profiles)
	if err != nil {
		t.Fatalf("ConvertTaggedStep: %v", err)
	}
	reversed := []ConcreteProfileSnapshot{profiles[2], profiles[1], profiles[0]}
	second, err := ConvertTaggedStep("step-1", []string{"claude"}, "", reversed)
	if err != nil {
		t.Fatalf("ConvertTaggedStep reversed: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("conversion differs with source order:\n%#v\n%#v", first, second)
	}
	// Only the two claude profiles match, ordered by canonical ID.
	want := []string{"agent-a", "agent-b"}
	if got := candidateIDs(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestConvertTaggedStepDeduplicatesFallback(t *testing.T) {
	profiles := []ConcreteProfileSnapshot{
		snapshot("a", "claude"),
		snapshot("fallback", "codex"),
	}
	conversion, err := ConvertTaggedStep("step-2", []string{"claude"}, "fallback", profiles)
	if err != nil {
		t.Fatalf("ConvertTaggedStep: %v", err)
	}
	want := []string{"agent-a", "agent-fallback"}
	if got := candidateIDs(conversion); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}

	// A fallback that is already a matching candidate appears once.
	alreadyMatching := []ConcreteProfileSnapshot{
		snapshot("a", "claude"),
		snapshot("b", "claude"),
	}
	deduped, err := ConvertTaggedStep("step-3", []string{"claude"}, "b", alreadyMatching)
	if err != nil {
		t.Fatalf("ConvertTaggedStep dedupe: %v", err)
	}
	if got := candidateIDs(deduped); !reflect.DeepEqual(got, []string{"agent-a", "agent-b"}) {
		t.Fatalf("candidates = %v, want fallback deduplicated", got)
	}
}

func TestConvertTaggedStepRejectsNoCandidate(t *testing.T) {
	_, err := ConvertTaggedStep("step-4", []string{"claude"}, "", []ConcreteProfileSnapshot{
		snapshot("a", "codex"),
	})
	if !errors.Is(err, ErrTaggedStepConversion) {
		t.Fatalf("error = %v, want ErrTaggedStepConversion", err)
	}

	// A fallback that is not present in the snapshot is not a valid candidate.
	_, err = ConvertTaggedStep("step-4", []string{"claude"}, "missing", []ConcreteProfileSnapshot{
		snapshot("a", "codex"),
	})
	if !errors.Is(err, ErrTaggedStepConversion) {
		t.Fatalf("error = %v, want ErrTaggedStepConversion for missing fallback", err)
	}

	// Empty allowed tags is rejected rather than converted.
	if _, err := ConvertTaggedStep("step-4", nil, "a", []ConcreteProfileSnapshot{snapshot("a")}); !errors.Is(err, ErrTaggedStepConversion) {
		t.Fatalf("empty tags error = %v, want ErrTaggedStepConversion", err)
	}
}

func TestConvertTaggedStepRepeatedConversionIsIdentical(t *testing.T) {
	profiles := []ConcreteProfileSnapshot{snapshot("a", "claude"), snapshot("b", "claude")}
	first, err := ConvertTaggedStep("step-5", []string{" claude "}, "", profiles)
	if err != nil {
		t.Fatalf("ConvertTaggedStep: %v", err)
	}
	second, err := ConvertTaggedStep("step-5", []string{"claude"}, "", profiles)
	if err != nil {
		t.Fatalf("ConvertTaggedStep repeat: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated conversion differs:\n%#v\n%#v", first, second)
	}
	if first.Dynamic.MigratedFrom != MigratedFromTaggedStepPrefix+"step-5" {
		t.Fatalf("migrated_from = %q", first.Dynamic.MigratedFrom)
	}
	if !reflect.DeepEqual(first.Dynamic.PreferredTags, []string{"claude"}) {
		t.Fatalf("preferred tags = %v", first.Dynamic.PreferredTags)
	}
	if len(first.Dynamic.AvoidedTags) != 0 {
		t.Fatalf("avoided tags = %v, want empty", first.Dynamic.AvoidedTags)
	}
	if first.AgentProfile.Dynamic == nil {
		t.Fatalf("rewritten binding has no dynamic descriptor")
	}
}

func TestConvertTaggedStepsStableAndNonMutating(t *testing.T) {
	export := &WorkflowExport{Version: ExportVersion, Type: ExportType, Workflows: []WorkflowPortable{{
		Name: "Review",
		Steps: []StepPortable{
			{Name: "Review", Position: 0, Color: "bg", AllowedTags: []string{"claude"}},
			{Name: "Done", Position: 1, Color: "bg"},
		},
	}}}
	profiles := []ConcreteProfileSnapshot{snapshot("a", "claude"), snapshot("b", "codex")}
	first, err := ConvertTaggedSteps(export, profiles, "")
	if err != nil {
		t.Fatalf("ConvertTaggedSteps: %v", err)
	}
	second, err := ConvertTaggedSteps(first.Export, profiles, "")
	if err != nil {
		t.Fatalf("ConvertTaggedSteps repeat: %v", err)
	}
	// Re-converting an already-generated descriptor bypasses conversion.
	if len(second.ByStep) != 0 {
		t.Fatalf("already-generated step was converted again: %#v", second.ByStep)
	}
	// The caller's document is untouched.
	if export.Workflows[0].Steps[0].AgentProfile != nil {
		t.Fatalf("input export was mutated")
	}
	// Exactly one legacy step converted, with allowed_tags retained.
	if len(first.ByStep) != 1 {
		t.Fatalf("conversions = %d, want 1", len(first.ByStep))
	}
	converted := first.Export.Workflows[0].Steps[0]
	if converted.AgentProfile == nil || converted.AgentProfile.Dynamic == nil {
		t.Fatalf("converted step has no dynamic descriptor")
	}
	if !reflect.DeepEqual(converted.AllowedTags, []string{"claude"}) {
		t.Fatalf("allowed_tags not retained: %v", converted.AllowedTags)
	}
}

func TestConvertTaggedStepsRejectsUnmatchableStep(t *testing.T) {
	export := &WorkflowExport{Version: ExportVersion, Type: ExportType, Workflows: []WorkflowPortable{{
		Name:  "Review",
		Steps: []StepPortable{{Name: "Review", Position: 0, Color: "bg", AllowedTags: []string{"claude"}}},
	}}}
	if _, err := ConvertTaggedSteps(export, nil, ""); !errors.Is(err, ErrTaggedStepConversion) {
		t.Fatalf("error = %v, want ErrTaggedStepConversion", err)
	}
}

func TestDynamicDescriptorRoundTripsPortableJSON(t *testing.T) {
	conversion, err := ConvertTaggedStep("step-6", []string{"claude"}, "", []ConcreteProfileSnapshot{
		snapshot("a", "claude"),
	})
	if err != nil {
		t.Fatalf("ConvertTaggedStep: %v", err)
	}
	step := StepPortable{Name: "Review", Position: 0, Color: "bg", AllowedTags: append([]string{}, conversion.Dynamic.PreferredTags...), AgentProfile: &conversion.AgentProfile}
	payload, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal step: %v", err)
	}
	var decoded StepPortable
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal step: %v", err)
	}
	if decoded.AgentProfile == nil || decoded.AgentProfile.Dynamic == nil {
		t.Fatalf("decoded binding missing dynamic descriptor: %#v", decoded.AgentProfile)
	}
	if !reflect.DeepEqual(decoded.AgentProfile.Dynamic, conversion.AgentProfile.Dynamic) {
		t.Fatalf("dynamic descriptor round-trip mismatch:\n%#v\n%#v", decoded.AgentProfile.Dynamic, conversion.AgentProfile.Dynamic)
	}
	if !reflect.DeepEqual(decoded.AllowedTags, []string{"claude"}) {
		t.Fatalf("allowed_tags not retained: %v", decoded.AllowedTags)
	}
}
