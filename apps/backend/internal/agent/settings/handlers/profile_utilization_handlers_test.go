package handlers

import (
	"context"
	"errors"
	"testing"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

type fakeUsageProvider struct {
	usage map[string]*agentusage.ProviderUsage
	errs  map[string]error
	calls int
}

func (f *fakeUsageProvider) GetUsage(_ context.Context, id string) (*agentusage.ProviderUsage, error) {
	f.calls++
	if err := f.errs[id]; err != nil {
		return nil, err
	}
	return f.usage[id], nil
}

func TestNormalizeProfileUtilizationIDs(t *testing.T) {
	got, err := normalizeProfileUtilizationIDs([]string{" b ", "a", "b", "c"})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if _, err := normalizeProfileUtilizationIDs(nil); err == nil {
		t.Fatal("empty ids should be rejected")
	}
	if _, err := normalizeProfileUtilizationIDs([]string{"a", " "}); err == nil {
		t.Fatal("blank id should be rejected")
	}
	tooMany := make([]string, maxProfileUtilizationIDs+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	if _, err := normalizeProfileUtilizationIDs(tooMany); err == nil {
		t.Fatal("more than the cap should be rejected")
	}
}

func TestCollectProfileUtilizationStates(t *testing.T) {
	provider := &fakeUsageProvider{
		usage: map[string]*agentusage.ProviderUsage{
			"known":   {Windows: []agentusage.UtilizationWindow{{UtilizationPct: 25}}},
			"unknown": nil,
		},
		errs: map[string]error{"unavailable": errors.New("boom")},
	}
	h := &Handlers{profileUsage: provider}
	items := h.collectProfileUtilization(context.Background(), []string{"known", "unknown", "unavailable"})

	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].ProfileID != "known" || items[0].State != "known" || items[0].RemainingPct == nil || *items[0].RemainingPct != 75 {
		t.Fatalf("known item = %#v", items[0])
	}
	if items[1].State != "unknown" || items[1].RemainingPct != nil {
		t.Fatalf("unknown item = %#v", items[1])
	}
	if items[2].State != "unavailable" {
		t.Fatalf("unavailable item = %#v", items[2])
	}
}

func TestCollectProfileUtilizationNilProviderIsUnknown(t *testing.T) {
	h := &Handlers{}
	items := h.collectProfileUtilization(context.Background(), []string{"a", "b"})
	for _, item := range items {
		if item.State != "unknown" {
			t.Fatalf("item = %#v, want unknown", item)
		}
	}
}
