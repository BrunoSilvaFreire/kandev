package usage

import "testing"

func TestRemainingPctUsesMaxWindow(t *testing.T) {
	usage := &ProviderUsage{Windows: []UtilizationWindow{
		{Label: "5-hour", UtilizationPct: 28},
		{Label: "7-day", UtilizationPct: 64},
	}}
	got, known := RemainingPct(usage)
	if !known || got != 36 {
		t.Fatalf("got (%v, %v), want (36, true)", got, known)
	}
}

func TestRemainingPctClamps(t *testing.T) {
	if got, known := RemainingPct(&ProviderUsage{Windows: []UtilizationWindow{{UtilizationPct: 140}}}); !known || got != 0 {
		t.Fatalf("got (%v, %v), want (0, true)", got, known)
	}
	if got, known := RemainingPct(&ProviderUsage{Windows: []UtilizationWindow{{UtilizationPct: -10}}}); !known || got != 100 {
		t.Fatalf("got (%v, %v), want (100, true)", got, known)
	}
}

func TestRemainingPctUnknown(t *testing.T) {
	if _, known := RemainingPct(nil); known {
		t.Fatal("nil usage should be unknown")
	}
	if _, known := RemainingPct(&ProviderUsage{}); known {
		t.Fatal("empty windows should be unknown")
	}
}
