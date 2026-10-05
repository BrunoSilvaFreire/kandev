package lifecycle

import (
	"sync"
	"testing"
	"time"
)

type recordedQuotaCall struct {
	profileID string
	agentID   string
	code      string
	resetHint time.Time
}

type recordingQuotaSignal struct {
	mu    sync.Mutex
	calls []recordedQuotaCall
}

func (r *recordingQuotaSignal) RecordLimitHit(profileID, agentID, code string, resetHint time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedQuotaCall{profileID, agentID, code, resetHint})
}

func (r *recordingQuotaSignal) snapshot() []recordedQuotaCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedQuotaCall(nil), r.calls...)
}

func TestClassifyAndMaybeRemediate_RecordsQuotaSignal(t *testing.T) {
	m := newRemediateTestManager(t)
	recorder := &recordingQuotaSignal{}
	m.SetQuotaSignalRecorder(recorder)

	exec := &AgentExecution{ID: "exec-q", AgentID: "claude-acp", AgentProfileID: "profile-q"}
	m.classifyAndMaybeRemediate(exec, 0, "API error: rate limit exceeded, please retry")

	calls := recorder.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 quota signal, got %d: %+v", len(calls), calls)
	}
	if calls[0].profileID != "profile-q" || calls[0].agentID != "claude-acp" {
		t.Fatalf("unexpected identity: %+v", calls[0])
	}
	if calls[0].code != "rate_limited" {
		t.Fatalf("expected rate_limited, got %q", calls[0].code)
	}
}

func TestClassifyAndMaybeRemediate_NoQuotaSignalForOtherFailures(t *testing.T) {
	m := newRemediateTestManager(t)
	recorder := &recordingQuotaSignal{}
	m.SetQuotaSignalRecorder(recorder)

	exec := &AgentExecution{ID: "exec-n", AgentID: "claude-acp", AgentProfileID: "profile-n"}
	m.classifyAndMaybeRemediate(exec, 1, "some unrelated stderr")

	if calls := recorder.snapshot(); len(calls) != 0 {
		t.Fatalf("unrelated failure must not record a quota signal: %+v", calls)
	}
}

func TestRecordQuotaSignalNilSafe(t *testing.T) {
	m := newRemediateTestManager(t)
	exec := &AgentExecution{ID: "exec-nil", AgentID: "claude-acp"}
	// No recorder installed: must not panic.
	m.classifyAndMaybeRemediate(exec, 0, "rate limit exceeded")
}
