package providerusage

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

const (
	overviewProfileCap   = 200
	liveFetchConcurrency = 8
	overviewMaxPoints    = 200
)

// Overview range keys.
const (
	Range24h = "24h"
	Range7d  = "7d"
	Range30d = "30d"
)

var rangeSpans = map[string]time.Duration{
	Range24h: 24 * time.Hour,
	Range7d:  7 * 24 * time.Hour,
	Range30d: 30 * 24 * time.Hour,
}

// providerDisplayNames gives the first-party providers a human-readable name
// without waiting for an agent-registry lookup.
var providerDisplayNames = map[string]string{
	ProviderAnthropic:   "Anthropic",
	ProviderOpenAI:      "OpenAI",
	ProviderAntigravity: "Antigravity",
	ProviderGoogle:      "Google",
	ProviderOpenCodeGo:  "OpenCode Go",
	ProviderJunie:       "Junie",
}

// localHistoryLabels names the backfill-only accounts that have no linked
// profile or plan; their identity is the local file they were read from.
var localHistoryLabels = map[string]string{
	SourceClaudeLocal: "Claude Code local history",
	SourceCodexLocal:  "Codex local history",
}

// ProfileRef identifies a profile sharing an account.
type ProfileRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// HistoryPoint is one point on a window's chart series.
type HistoryPoint struct {
	At   time.Time `json:"at"`
	Pct  float64   `json:"pct"`
	Kind string    `json:"kind"` // measured | estimated | limit_hit
}

// WindowView is one account window with its estimate and history.
type WindowView struct {
	Label          string         `json:"label"`
	UtilizationPct float64        `json:"utilization_pct"`
	ResetAt        time.Time      `json:"reset_at,omitempty"`
	Source         string         `json:"source"` // measured | estimated
	Estimate       *Estimate      `json:"estimate,omitempty"`
	History        []HistoryPoint `json:"history"`
}

// Credential kinds. They are a closed set the frontend maps to translated
// instructions.
const (
	CredentialKindOpenCodeConsoleCookie = "opencode_console_cookie"
	CredentialKindJunieAPIKey           = "junie_api_key"
)

// CredentialHint names the Kandev global secret that enables or refreshes this
// account's live quota.
type CredentialHint struct {
	Kind       string `json:"kind"`       // closed set: opencode_console_cookie | junie_api_key
	AgentType  string `json:"agent_type"` // "opencode-acp" | "junie-acp"
	SecretName string `json:"secret_name"`
	Configured bool   `json:"configured"`
}

// ProviderAccount is one deduplicated provider credential.
type ProviderAccount struct {
	AccountKey             string                   `json:"account_key"`
	Label                  string                   `json:"label,omitempty"`
	Plan                   string                   `json:"plan,omitempty"`
	Status                 string                   `json:"status"`
	QuotaUnavailableReason string                   `json:"quota_unavailable_reason,omitempty"`
	Profiles               []ProfileRef             `json:"profiles"`
	Windows                []WindowView             `json:"windows"`
	Balances               []agentusage.Balance     `json:"balances,omitempty"`
	Subscription           *agentusage.Subscription `json:"subscription,omitempty"`
	Credential             *CredentialHint          `json:"credential,omitempty"`
}

// ProviderView groups accounts under one provider.
type ProviderView struct {
	Provider    string            `json:"provider"`
	DisplayName string            `json:"display_name,omitempty"`
	Status      string            `json:"status"`
	Accounts    []ProviderAccount `json:"accounts"`
}

// SourceView is one history source's consent state.
type SourceView struct {
	Source     string `json:"source"`
	Enabled    bool   `json:"enabled"`
	Toggleable bool   `json:"toggleable"`
}

// OverviewResponse is the GET /api/v1/provider-usage payload.
type OverviewResponse struct {
	Range     string         `json:"range"`
	Truncated bool           `json:"truncated"`
	Indexing  IndexStatus    `json:"indexing"`
	Sources   []SourceView   `json:"sources"`
	Providers []ProviderView `json:"providers"`
}

type listedProfile struct {
	id   string
	name string
	// agentType is the agent *type* name from the settings agent record (for
	// example "codex-acp"), never the profile's agents.id UUID.
	agentType string
	agentName string
	model     string
}

type liveResult struct {
	usage  *agentusage.ProviderUsage
	failed bool
}

type accountAccumulator struct {
	accountKey    string
	provider      string
	displayName   string
	plan          string
	profiles      []ProfileRef
	liveUsage     *agentusage.ProviderUsage
	liveSucceeded bool
	liveFailed    bool
}

// Overview builds the account-wide provider usage projection.
func (s *Service) Overview(ctx context.Context, rangeKey string) (*OverviewResponse, error) {
	canonical, span := parseRange(rangeKey)
	now := time.Now().UTC()
	since := now.Add(-span)

	sources, err := s.repo.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	profiles, truncated, err := s.listProfiles(ctx)
	if err != nil {
		return nil, err
	}
	live := s.fetchLive(ctx, profiles)
	grouped := s.groupAccounts(profiles, live)

	providers := make([]ProviderView, 0, len(grouped))
	for provider, accounts := range grouped {
		view := s.buildProvider(ctx, provider, accounts, since, now)
		providers = append(providers, view)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Provider < providers[j].Provider })

	return &OverviewResponse{
		Range:     canonical,
		Truncated: truncated,
		Indexing:  s.indexer.Status(),
		Sources:   sourceViews(sources),
		Providers: providers,
	}, nil
}

func parseRange(key string) (string, time.Duration) {
	if span, ok := rangeSpans[key]; ok {
		return key, span
	}
	return Range7d, rangeSpans[Range7d]
}

// providerDisplayName resolves the human-readable provider name: a canonical
// name for the first-party providers, else the agent registry's display name
// for a dynamic agent, else the raw id.
func providerDisplayName(provider, agentName string) string {
	if name, ok := providerDisplayNames[provider]; ok {
		return name
	}
	if agentName != "" {
		return agentName
	}
	return provider
}

// accountLabel names an account from what is known about it, preferring the
// plan plus the first linked profile and falling back to the local history
// source for backfill-only accounts. Never returns the account key.
func accountLabel(plan string, profiles []ProfileRef, observations []Observation) string {
	name := firstProfileName(profiles)
	switch {
	case plan != "" && name != "":
		return plan + " \u00b7 " + name
	case plan != "":
		return plan
	case name != "":
		return name
	}
	for _, source := range []string{SourceClaudeLocal, SourceCodexLocal} {
		if hasObservationSource(observations, source) {
			return localHistoryLabels[source]
		}
	}
	return ""
}

func firstProfileName(profiles []ProfileRef) string {
	for _, profile := range profiles {
		if profile.Name != "" {
			return profile.Name
		}
	}
	return ""
}

func hasObservationSource(observations []Observation, source string) bool {
	for _, obs := range observations {
		if obs.Source == source {
			return true
		}
	}
	return false
}

func sourceViews(sources map[string]Source) []SourceView {
	views := []SourceView{{Source: SourceKandev, Enabled: true, Toggleable: false}}
	for _, source := range ToggleableSources {
		views = append(views, SourceView{Source: source, Enabled: sources[source].Enabled, Toggleable: true})
	}
	return views
}

func (s *Service) listProfiles(ctx context.Context) ([]listedProfile, bool, error) {
	agents, err := s.profiles.ListAgents(ctx)
	if err != nil {
		return nil, false, err
	}
	out := make([]listedProfile, 0, len(agents))
	truncated := false
	for _, agent := range agents {
		profiles, err := s.profiles.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			continue
		}
		for _, profile := range profiles {
			if len(out) >= overviewProfileCap {
				truncated = true
				break
			}
			agentName := profile.AgentDisplayName
			if agentName == "" {
				agentName = agent.Name
			}
			out = append(out, listedProfile{
				id:        profile.ID,
				name:      profile.Name,
				agentType: agent.Name,
				agentName: agentName,
				model:     profile.Model,
			})
		}
		if truncated {
			break
		}
	}
	return out, truncated, nil
}

func (s *Service) fetchLive(ctx context.Context, profiles []listedProfile) map[string]liveResult {
	results := make(map[string]liveResult, len(profiles))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, liveFetchConcurrency)
	for _, profile := range profiles {
		wg.Add(1)
		sem <- struct{}{}
		go func(profile listedProfile) {
			defer wg.Done()
			defer func() { <-sem }()
			usage, err := s.live.GetUsage(ctx, profile.id)
			mu.Lock()
			results[profile.id] = liveResult{usage: usage, failed: err != nil}
			mu.Unlock()
		}(profile)
	}
	wg.Wait()
	return results
}

func (s *Service) groupAccounts(profiles []listedProfile, live map[string]liveResult) map[string]map[string]*accountAccumulator {
	grouped := make(map[string]map[string]*accountAccumulator)
	for _, profile := range profiles {
		res := live[profile.id]
		provider := providerForProfile(profile.agentType, profile.model)
		if res.usage != nil && res.usage.Provider != "" {
			provider = res.usage.Provider
		}
		key, ok := s.live.CacheKeyFor(profile.id)
		if !ok || key == "" {
			key = AccountProfilePrefix + profile.id
		}
		accounts := grouped[provider]
		if accounts == nil {
			accounts = make(map[string]*accountAccumulator)
			grouped[provider] = accounts
		}
		acc := accounts[key]
		if acc == nil {
			acc = &accountAccumulator{
				accountKey:  key,
				provider:    provider,
				displayName: providerDisplayName(provider, profile.agentName),
			}
			accounts[key] = acc
		}
		acc.profiles = append(acc.profiles, ProfileRef{ID: profile.id, Name: profile.name})
		if res.usage != nil {
			acc.liveSucceeded = true
			acc.liveUsage = res.usage
			if res.usage.Plan != "" {
				acc.plan = res.usage.Plan
			}
		}
		if res.failed {
			acc.liveFailed = true
		}
	}
	return grouped
}

func (s *Service) buildProvider(ctx context.Context, provider string, accounts map[string]*accountAccumulator, since, now time.Time) ProviderView {
	view := ProviderView{Provider: provider, DisplayName: provider}
	statuses := make([]string, 0, len(accounts))
	keys := make([]string, 0, len(accounts))
	for key, acc := range accounts {
		keys = append(keys, key)
		if acc.displayName != "" {
			view.DisplayName = acc.displayName
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		account := s.buildAccount(ctx, accounts[key], since, now)
		view.Accounts = append(view.Accounts, account)
		statuses = append(statuses, account.Status)
	}
	view.Status = WorstStatus(statuses)
	return view
}

func (s *Service) buildAccount(ctx context.Context, acc *accountAccumulator, since, now time.Time) ProviderAccount {
	account := ProviderAccount{
		AccountKey: acc.accountKey,
		Label:      accountLabel(acc.plan, acc.profiles, nil),
		Plan:       acc.plan,
		Profiles:   acc.profiles,
		Credential: s.quotaCredential(ctx, acc),
	}
	if acc.liveUsage != nil {
		account.Balances = acc.liveUsage.Balances
		account.Subscription = acc.liveUsage.Subscription
	}
	observations, err := s.repo.ListObservations(ctx, acc.accountKey, since)
	if err != nil {
		account.Status = StatusUnavailable
		account.QuotaUnavailableReason = s.quotaUnavailableReason(ctx, acc)
		return account
	}
	account.Label = accountLabel(acc.plan, acc.profiles, observations)
	measured := measuredLabels(acc, observations)
	windows := make([]WindowView, 0, len(measured))
	for _, label := range measuredLabelsOrder(measured) {
		windows = append(windows, s.buildWindow(label, observations, measured[label], now))
	}
	if acc.provider == ProviderAnthropic && acc.liveUsage != nil {
		appendClaudeEstimates(observations, windows)
	}
	account.Windows = windows
	account.Status = s.accountStatus(acc, measured, observations, now)
	if len(windows) == 0 {
		account.QuotaUnavailableReason = s.quotaUnavailableReason(ctx, acc)
	}
	return account
}

// appendClaudeEstimates adds a calibrated estimated series to each live Claude
// window. It uses the Claude transcript token series when present, else the
// Kandev ledger series (dedupe rule).
func appendClaudeEstimates(observations []Observation, windows []WindowView) {
	tokens := tokenBuckets(observations)
	if len(tokens) == 0 {
		return
	}
	for i := range windows {
		window := &windows[i]
		if window.ResetAt.IsZero() {
			continue
		}
		duration := windowDuration(window.Label)
		if duration == 0 {
			continue
		}
		start := window.ResetAt.Add(-duration)
		var inWindow []TokenBucket
		var total int64
		for _, bucket := range tokens {
			if !bucket.At.Before(start) {
				inWindow = append(inWindow, bucket)
				total += bucket.WeightedTokens
			}
		}
		if total == 0 {
			continue
		}
		factor := Calibration(window.UtilizationPct, float64(total))
		for _, point := range EstimatedSeries(inWindow, factor) {
			window.History = append(window.History, HistoryPoint{At: point.At, Pct: point.UtilPct, Kind: point.Kind})
		}
		window.History = downsample(window.History, overviewMaxPoints)
	}
}

func tokenBuckets(observations []Observation) []TokenBucket {
	useClaudeLocal := false
	for _, obs := range observations {
		if obs.Kind == KindTokens && obs.Source == SourceClaudeLocal {
			useClaudeLocal = true
			break
		}
	}
	var buckets []TokenBucket
	for _, obs := range observations {
		if obs.Kind != KindTokens || obs.WeightedTokens == nil {
			continue
		}
		if useClaudeLocal && obs.Source != SourceClaudeLocal {
			continue
		}
		if !useClaudeLocal && obs.Source != SourceKandev {
			continue
		}
		buckets = append(buckets, TokenBucket{At: obs.ObservedAt, WeightedTokens: *obs.WeightedTokens})
	}
	return buckets
}

func measuredLabels(acc *accountAccumulator, observations []Observation) map[string]WindowUtil {
	windows := make(map[string]WindowUtil)
	if acc.liveUsage != nil {
		for _, w := range acc.liveUsage.Windows {
			windows[w.Label] = WindowUtil{Label: w.Label, UtilizationPct: w.UtilizationPct, ResetAt: w.ResetAt, Source: "measured"}
		}
		return windows
	}
	// Keep the newest reading per label. Observations arrive ascending, but
	// compare explicitly so the stale fallback cannot serve an older value.
	latest := make(map[string]time.Time, len(windows))
	for _, obs := range observations {
		if obs.Kind != KindMeasured || obs.UtilizationPct == nil {
			continue
		}
		if seen, ok := latest[obs.WindowLabel]; ok && !obs.ObservedAt.After(seen) {
			continue
		}
		latest[obs.WindowLabel] = obs.ObservedAt
		windows[obs.WindowLabel] = WindowUtil{
			Label: obs.WindowLabel, UtilizationPct: *obs.UtilizationPct,
			ResetAt: obs.ResetAt, Source: "measured",
		}
	}
	return windows
}

func measuredLabelsOrder(windows map[string]WindowUtil) []string {
	labels := make([]string, 0, len(windows))
	for label := range windows {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

func (s *Service) buildWindow(label string, observations []Observation, window WindowUtil, now time.Time) WindowView {
	measuredPoints := pointsForLabel(observations, label)
	estimate := EstimateWindow(measuredPoints, now)
	history := hourlyMax(measuredPoints)
	history = append(history, limitHitPoints(observations)...)
	return WindowView{
		Label:          label,
		UtilizationPct: window.UtilizationPct,
		ResetAt:        window.ResetAt,
		Source:         "measured",
		Estimate:       &estimate,
		History:        downsample(history, overviewMaxPoints),
	}
}

// quotaUnavailableReason asks the live provider why an account has no quota
// source. Accounts with no linked profile can only be local history, which has
// no provider endpoint.
func (s *Service) quotaUnavailableReason(ctx context.Context, acc *accountAccumulator) string {
	if s.live == nil || len(acc.profiles) == 0 {
		return QuotaUnavailableNoProviderEndpoint
	}
	return s.live.QuotaUnavailableReason(ctx, acc.profiles[0].ID)
}

// quotaCredential resolves the credential hint for an account, or nil when the
// provider needs none.
func (s *Service) quotaCredential(ctx context.Context, acc *accountAccumulator) *CredentialHint {
	if s.live == nil || len(acc.profiles) == 0 {
		return nil
	}
	return s.live.QuotaCredential(ctx, acc.profiles[0].ID)
}

// liveAccountState reports whether the live provider told us the account is
// usable even without utilization windows: a positive prepaid balance or an
// active subscription.
func liveAccountState(usage *agentusage.ProviderUsage) bool {
	if usage == nil {
		return false
	}
	for _, balance := range usage.Balances {
		if balance.Amount > 0 {
			return true
		}
	}
	if usage.Subscription == nil {
		return false
	}
	switch usage.Subscription.Status {
	case "active", "canceling", "renewal_pending":
		return true
	default:
		return false
	}
}

func (s *Service) accountStatus(acc *accountAccumulator, measured map[string]WindowUtil, observations []Observation, now time.Time) string {
	windows := make([]WindowUtil, 0, len(measured))
	for _, window := range measured {
		windows = append(windows, window)
	}
	last, _ := lastMeasuredAt(observations)
	hasSource := acc.liveSucceeded || acc.liveFailed
	if !hasSource {
		for _, obs := range observations {
			if obs.Kind == KindMeasured || obs.Kind == KindLimitHit {
				hasSource = true
				break
			}
		}
	}
	return AccountStatus(StatusInput{
		Windows:             windows,
		HasLimitHit:         activeLimitHit(observations, now),
		LiveFetchFailed:     acc.liveFailed && !acc.liveSucceeded,
		HasSource:           hasSource,
		LastObservedAt:      last,
		Now:                 now,
		HasLiveAccountState: liveAccountState(acc.liveUsage),
	})
}

func pointsForLabel(observations []Observation, label string) []Point {
	var points []Point
	for _, obs := range observations {
		if obs.Kind != KindMeasured || obs.UtilizationPct == nil || obs.WindowLabel != label {
			continue
		}
		points = append(points, Point{
			UtilPct: *obs.UtilizationPct, At: obs.ObservedAt, ResetAt: obs.ResetAt, Kind: KindMeasured,
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	return points
}

func limitHitPoints(observations []Observation) []HistoryPoint {
	var points []HistoryPoint
	for _, obs := range observations {
		if obs.Kind == KindLimitHit {
			points = append(points, HistoryPoint{At: obs.ObservedAt, Pct: exhaustedPct, Kind: KindLimitHit})
		}
	}
	return points
}

func activeLimitHit(observations []Observation, now time.Time) bool {
	for _, obs := range observations {
		if obs.Kind != KindLimitHit {
			continue
		}
		if obs.ResetAt.After(now) {
			return true
		}
		if obs.ResetAt.IsZero() && obs.ObservedAt.Add(limitHitTTL).After(now) {
			return true
		}
	}
	return false
}

func lastMeasuredAt(observations []Observation) (time.Time, bool) {
	var last time.Time
	for _, obs := range observations {
		if obs.Kind == KindMeasured && obs.ObservedAt.After(last) {
			last = obs.ObservedAt
		}
	}
	return last, !last.IsZero()
}

// hourlyMax down-samples a series to one point per hour, keeping the highest
// utilization in each hour.
func hourlyMax(points []Point) []HistoryPoint {
	order := make([]time.Time, 0)
	byHour := make(map[time.Time]float64)
	for _, point := range points {
		hour := point.At.UTC().Truncate(time.Hour)
		if _, ok := byHour[hour]; !ok {
			order = append(order, hour)
		}
		if point.UtilPct > byHour[hour] {
			byHour[hour] = point.UtilPct
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
	history := make([]HistoryPoint, 0, len(order))
	for _, hour := range order {
		history = append(history, HistoryPoint{At: hour, Pct: byHour[hour], Kind: KindMeasured})
	}
	return history
}

// downsample caps a series at max points by stride, preserving order.
func downsample(points []HistoryPoint, max int) []HistoryPoint {
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })
	if len(points) <= max || max <= 0 {
		return points
	}
	stride := (len(points) + max - 1) / max
	out := make([]HistoryPoint, 0, max)
	for i := 0; i < len(points); i += stride {
		out = append(out, points[i])
	}
	return out
}

// windowDuration parses a live window label such as "5-hour" or "7-day".
func windowDuration(label string) time.Duration {
	parts := strings.SplitN(label, "-", 2)
	if len(parts) != 2 {
		return 0
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	switch parts[1] {
	case "hour":
		return time.Duration(count) * time.Hour
	case "day":
		return time.Duration(count) * 24 * time.Hour
	default:
		return 0
	}
}
