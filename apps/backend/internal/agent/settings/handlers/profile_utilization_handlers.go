package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// maxProfileUtilizationIDs bounds one batch utilization request so a caller can
// never turn settings into an unbounded fan-out of provider calls.
const maxProfileUtilizationIDs = 50

// maxProfileUtilizationConcurrency bounds in-flight provider fetches per request.
const maxProfileUtilizationConcurrency = 8

// profileUtilizationState* are the batch endpoint's state values.
const (
	profileUtilizationStateKnown       = "known"
	profileUtilizationStateUnknown     = "unknown"
	profileUtilizationStateUnavailable = "unavailable"
)

// ProfileUsageProvider fetches subscription utilization for one agent profile.
// A nil provider degrades every item to unknown rather than failing the request.
type ProfileUsageProvider interface {
	GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error)
}

// ProfileUsageModelProvider supplies the configured model when a usage
// provider can resolve it. It is optional to preserve unknown semantics for
// alternate providers.
type ProfileUsageModelProvider interface {
	ProfileModel(ctx context.Context, profileID string) (string, error)
}

// SetProfileUsageProvider wires the shared usage adapter into the batch
// utilization endpoint. Safe to leave unset; the endpoint then reports unknown.
func (h *Handlers) SetProfileUsageProvider(provider ProfileUsageProvider) {
	h.profileUsage = provider
}

type profileUtilizationRequest struct {
	ProfileIDs []string `json:"profile_ids"`
}

type profileUtilizationItem struct {
	ProfileID    string                    `json:"profile_id"`
	State        string                    `json:"state"`
	RemainingPct *float64                  `json:"remaining_pct,omitempty"`
	Utilization  *agentusage.ProviderUsage `json:"utilization,omitempty"`
}

type profileUtilizationResponse struct {
	Profiles []profileUtilizationItem `json:"profiles"`
}

// httpProfileUtilization returns bounded, deterministic subscription
// utilization for the requested concrete profiles. Provider errors are
// candidate-local: one unavailable profile never fails the batch.
func (h *Handlers) httpProfileUtilization(c *gin.Context) {
	var body profileUtilizationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	ids, err := normalizeProfileUtilizationIDs(body.ProfileIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items := h.collectProfileUtilization(c.Request.Context(), ids)
	c.JSON(http.StatusOK, profileUtilizationResponse{Profiles: items})
}

func normalizeProfileUtilizationIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("profile_ids is required")
	}
	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, fmt.Errorf("profile_ids must not contain empty values")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > maxProfileUtilizationIDs {
		return nil, fmt.Errorf("at most %d profile ids allowed", maxProfileUtilizationIDs)
	}
	sort.Strings(ids)
	return ids, nil
}

func (h *Handlers) collectProfileUtilization(ctx context.Context, ids []string) []profileUtilizationItem {
	items := make([]profileUtilizationItem, len(ids))
	sem := make(chan struct{}, maxProfileUtilizationConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			items[i] = h.profileUtilizationItem(ctx, id)
		}(i, id)
	}
	wg.Wait()
	return items
}

func (h *Handlers) profileUtilizationItem(ctx context.Context, id string) profileUtilizationItem {
	item := profileUtilizationItem{ProfileID: id, State: profileUtilizationStateUnknown}
	if h.profileUsage == nil {
		return item
	}
	usage, err := h.profileUsage.GetUsage(ctx, id)
	if err != nil {
		item.State = profileUtilizationStateUnavailable
		return item
	}
	model := ""
	if provider, ok := h.profileUsage.(ProfileUsageModelProvider); ok {
		model, _ = provider.ProfileModel(ctx, id)
	}
	remaining, known := agentusage.RemainingPct(usage, model)
	if !known {
		return item
	}
	item.State = profileUtilizationStateKnown
	item.RemainingPct = &remaining
	item.Utilization = usage
	return item
}
