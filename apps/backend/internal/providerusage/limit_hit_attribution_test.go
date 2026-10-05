package providerusage

import (
	"context"
	"testing"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// TestRecordLimitHitOpenCodeGoUsesGoAccountKey proves a Go limit hit is stored
// under the account-wide Go key with the opencode-go provider, even when the
// agent identifier is a UUID. The previous string-prefix check never matched
// the hashed cache key, so this hit was unattributed.
func TestRecordLimitHitOpenCodeGoUsesGoAccountKey(t *testing.T) {
	repo := newTestRepository(t)
	live := &fakeLive{keys: map[string]string{"p-go": agentusage.OpenCodeGoCacheKey()}}
	svc := NewService(repo, live, nil, Accounts{}, nil, testLogger(t))

	svc.recordLimitHit(context.Background(), "p-go", "53ebb87c-uuid", "quota", time.Now().UTC().Add(time.Hour))

	rows, err := repo.ListObservations(context.Background(), agentusage.OpenCodeGoCacheKey(), time.Time{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1: %+v", len(rows), rows)
	}
	if rows[0].Provider != ProviderOpenCodeGo {
		t.Fatalf("provider = %q, want %q", rows[0].Provider, ProviderOpenCodeGo)
	}
	if rows[0].Kind != KindLimitHit {
		t.Fatalf("kind = %q, want %q", rows[0].Kind, KindLimitHit)
	}
}
