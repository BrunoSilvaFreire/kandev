package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/providerusage"
)

// TestQuotaUnavailableReasonBranches covers every skip branch of the live
// quota lookup as a closed-set reason the frontend can translate.
func TestQuotaUnavailableReasonBranches(t *testing.T) {
	cases := []struct {
		name          string
		profileFound  bool
		agentFound    bool
		subscription  bool
		proxyResolved bool
		want          string
	}{
		{"profile missing", false, false, false, false, providerusage.QuotaUnavailableProfileMissing},
		{"agent has no endpoint", true, false, false, false, providerusage.QuotaUnavailableNoProviderEndpoint},
		{"api key billing", true, true, false, false, providerusage.QuotaUnavailableAPIKeyBilling},
		{"subscription has a source", true, true, true, false, ""},
		{"proxy source wins", true, true, false, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quotaUnavailableReasonFor(tc.profileFound, tc.agentFound, tc.subscription, tc.proxyResolved)
			if got != tc.want {
				t.Fatalf("quotaUnavailableReasonFor() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestQuotaUnavailableReasonProfileLookupFailure proves the adapter surfaces
// the profile-missing reason instead of panicking on an unavailable store.
func TestQuotaUnavailableReasonProfileLookupFailure(t *testing.T) {
	adapter := &usageProviderAdapter{settingsStore: failingProfileStore{}}
	if got := adapter.QuotaUnavailableReason(context.Background(), "missing"); got != providerusage.QuotaUnavailableProfileMissing {
		t.Fatalf("reason = %q, want %q", got, providerusage.QuotaUnavailableProfileMissing)
	}
}
