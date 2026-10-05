package backendapp

import (
	"context"
	"sync"
	"testing"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// staticUsageClient returns a fixed usage value, standing in for a live
// provider fetch.
type staticUsageClient struct{ usage *agentusage.ProviderUsage }

func (c staticUsageClient) FetchUsage(context.Context) (*agentusage.ProviderUsage, error) {
	return c.usage, nil
}

func countLimitWindows(u *agentusage.ProviderUsage) int {
	n := 0
	for _, w := range u.Windows {
		if w.Label == "limit" {
			n++
		}
	}
	return n
}

// TestWithLimitHitDoesNotMutateCachedUsage proves the synthetic limit window is
// appended to a per-call copy: repeated and concurrent readers each see exactly
// one synthetic window while the shared cached value stays untouched. Run with
// -race to cover the concurrent-reader case.
func TestWithLimitHitDoesNotMutateCachedUsage(t *testing.T) {
	now := time.Now()
	cached := &agentusage.ProviderUsage{
		Provider:  "openai",
		FetchedAt: now.Add(-time.Minute),
		Windows:   []agentusage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 10}},
	}
	svc := agentusage.NewUsageService()
	svc.Register("p", staticUsageClient{usage: cached}, "openai:key")
	adapter := &usageProviderAdapter{
		svc:       svc,
		limitHits: fakeLimitHits{observed: now, reset: now.Add(time.Hour), ok: true},
	}

	const readers = 8
	var wg sync.WaitGroup
	results := make([]*agentusage.ProviderUsage, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, err := adapter.getUsageWithLimitHit(context.Background(), "p")
			if err != nil {
				t.Errorf("getUsageWithLimitHit: %v", err)
			}
			results[i] = got
		}(i)
	}
	wg.Wait()

	for i, got := range results {
		if got == nil {
			t.Fatalf("reader %d got nil usage", i)
		}
		if n := countLimitWindows(got); n != 1 {
			t.Fatalf("reader %d saw %d synthetic windows, want 1: %#v", i, n, got.Windows)
		}
	}
	if n := countLimitWindows(cached); n != 0 {
		t.Fatalf("cached usage gained %d synthetic windows", n)
	}
	if len(cached.Windows) != 1 {
		t.Fatalf("cached windows = %d, want 1", len(cached.Windows))
	}
}
