package usage

import "testing"

func TestRemainingPctUsesMaxWindow(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "5-hour", UtilizationPct: 28},
		{Label: "7-day", UtilizationPct: 64},
	}}
	got, known := RemainingPct(usage, "")
	if !known || got != 36 {
		t.Fatalf("got (%v, %v), want (36, true)", got, known)
	}
}

func TestRemainingPctClamps(t *testing.T) {
	if got, known := RemainingPct(&ProviderUsage{Windows: []UtilizationWindow{{UtilizationPct: 140}}}, ""); !known || got != 0 {
		t.Fatalf("got (%v, %v), want (0, true)", got, known)
	}
	if got, known := RemainingPct(&ProviderUsage{Windows: []UtilizationWindow{{UtilizationPct: -10}}}, ""); !known || got != 100 {
		t.Fatalf("got (%v, %v), want (100, true)", got, known)
	}
}

func TestRemainingPctUnknown(t *testing.T) {
	if _, known := RemainingPct(nil, ""); known {
		t.Fatal("nil usage should be unknown")
	}
	if _, known := RemainingPct(&ProviderUsage{}, ""); known {
		t.Fatal("empty windows should be unknown")
	}
}

// TestRemainingPctFiltersOtherModels proves a bucket that names a model only
// affects the profile whose model it applies to.
func TestRemainingPctFiltersOtherModels(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "gemini-pro", Model: "gemini-2.5-pro", UtilizationPct: 95},
		{Label: "gemini-flash", Model: "gemini-2.5-flash", UtilizationPct: 10},
	}}
	got, known := RemainingPct(usage, "gemini-2.5-flash")
	if !known || got != 90 {
		t.Fatalf("flash profile got (%v, %v), want (90, true)", got, known)
	}
	got, known = RemainingPct(usage, "gemini-2.5-pro")
	if !known || got != 5 {
		t.Fatalf("pro profile got (%v, %v), want (5, true)", got, known)
	}
}

// TestRemainingPctPrefixMatchesModel covers a profile model that is a longer
// variant of the bucket's model id.
func TestRemainingPctPrefixMatchesModel(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "gemini-pro", Model: "gemini-2.5-pro", UtilizationPct: 40},
	}}
	got, known := RemainingPct(usage, "gemini-2.5-pro-preview")
	if !known || got != 60 {
		t.Fatalf("got (%v, %v), want (60, true)", got, known)
	}
}

// TestRemainingPctEmptyModelWindowsAlwaysApply: an unscoped window (Codex,
// Claude, a provider with no model breakdown) still applies to a model-scoped
// profile.
func TestRemainingPctEmptyModelWindowsAlwaysApply(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "5-hour", UtilizationPct: 30},
		{Label: "gemini-pro", Model: "gemini-2.5-pro", UtilizationPct: 95},
	}}
	got, known := RemainingPct(usage, "gemini-2.5-flash")
	if !known || got != 70 {
		t.Fatalf("got (%v, %v), want (70, true)", got, known)
	}
}

// TestRemainingPctEmptyModelConsidersAllWindows reproduces the pre-model
// behavior: an empty model argument applies every window, scoped or not.
func TestRemainingPctEmptyModelConsidersAllWindows(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "gemini-pro", Model: "gemini-2.5-pro", UtilizationPct: 95},
		{Label: "gemini-flash", Model: "gemini-2.5-flash", UtilizationPct: 10},
	}}
	got, known := RemainingPct(usage, "")
	if !known || got != 5 {
		t.Fatalf("got (%v, %v), want (5, true)", got, known)
	}
}

// TestRemainingPctAllWindowsFilteredIsUnknown: a profile whose model matches no
// bucket is unknown, never full or zero.
func TestRemainingPctAllWindowsFilteredIsUnknown(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "gemini-pro", Model: "gemini-2.5-pro", UtilizationPct: 95},
	}}
	if _, known := RemainingPct(usage, "some-other-model"); known {
		t.Fatal("a model with no matching bucket should be unknown")
	}
}
