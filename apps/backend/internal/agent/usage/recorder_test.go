package usage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
)

func TestUsageService_RecorderFiresOnlyOnRealFetch(t *testing.T) {
	svc := usage.NewUsageService()
	now := time.Now()
	mock := &mockClient{usage: &usage.ProviderUsage{
		Provider:  "anthropic",
		Windows:   []usage.UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now.Add(time.Hour)}},
		FetchedAt: now,
	}}
	key := usage.CacheKey("anthropic", "/recorder-creds")
	svc.Register("profile-rec", mock, key)

	var keys []string
	var count int
	svc.SetRecorder(func(k string, u *usage.ProviderUsage) {
		keys = append(keys, k)
		count++
	})

	if _, err := svc.GetUsage(context.Background(), "profile-rec"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 recorder call after fetch, got %d", count)
	}
	if keys[0] != key {
		t.Fatalf("expected key %q, got %q", key, keys[0])
	}

	// A cache hit must not fire the recorder.
	if _, err := svc.GetUsage(context.Background(), "profile-rec"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected still 1 recorder call after cache hit, got %d", count)
	}
	if mock.calls != 1 {
		t.Fatalf("expected 1 fetch call, got %d", mock.calls)
	}
}

func TestUsageService_RecorderNotFiredOnErrorOrNil(t *testing.T) {
	svc := usage.NewUsageService()
	svc.SetRecorder(func(string, *usage.ProviderUsage) {
		t.Fatal("recorder must not fire for an error or nil fetch")
	})

	errMock := &mockClient{err: errors.New("boom")}
	svc.Register("profile-err", errMock, usage.CacheKey("anthropic", "/err"))
	if _, err := svc.GetUsage(context.Background(), "profile-err"); err == nil {
		t.Fatal("expected error from failing client")
	}

	nilMock := &mockClient{}
	svc.Register("profile-nil", nilMock, usage.CacheKey("anthropic", "/nil"))
	if usage, err := svc.GetUsage(context.Background(), "profile-nil"); err != nil || usage != nil {
		t.Fatalf("expected nil usage and nil error, got %+v, %v", usage, err)
	}
}

func TestUsageService_CacheKeyFor(t *testing.T) {
	svc := usage.NewUsageService()
	key := usage.CacheKey("openai", "/codex-auth")
	svc.Register("profile-key", &mockClient{}, key)

	got, ok := svc.CacheKeyFor("profile-key")
	if !ok || got != key {
		t.Fatalf("expected %q,true; got %q,%v", key, got, ok)
	}
	if _, ok := svc.CacheKeyFor("missing"); ok {
		t.Fatal("expected false for an unregistered profile")
	}
}
