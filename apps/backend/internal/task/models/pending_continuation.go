package models

// Continuation cases recorded on a PendingContinuation. They name which
// automatic continuation path paused and therefore how a retry re-runs it.
const (
	PendingContinuationCaseCold          = "cold"
	PendingContinuationCaseUnavailable   = "unavailable"
	PendingContinuationCaseQuotaPressure = "quota_pressure"
)

// Why an automatic continuation paused.
const (
	PendingContinuationReasonExtractionFailed   = "extraction_failed"
	PendingContinuationReasonResetFailed        = "reset_failed"
	PendingContinuationReasonNoAvailableProfile = "no_available_profile"
)

// PendingContinuation is the single-slot, server-owned record of an automatic
// continuation that paused instead of sending its entry prompt. It is stored
// under MetaKeyPendingContinuation and redacted from public metadata. The stamp
// is a fresh unique value minted on every write; the resolver uses it as a
// compare-and-clear key so a second answer, or a later step entry, is a no-op.
type PendingContinuation struct {
	Stamp string `json:"stamp"`
	// StepID and EntryID identify the workflow entry that paused, so a later
	// entry for the same task can clear a stale record.
	StepID  string `json:"step_id"`
	EntryID string `json:"entry_id,omitempty"`
	// Case is one of PendingContinuationCase*.
	Case string `json:"case"`
	// SourceSessionID is the session whose continuation paused.
	SourceSessionID string `json:"source_session_id,omitempty"`
	// Reason is one of PendingContinuationReason*.
	Reason string `json:"reason"`
	// EntryPrompt is the exact prompt the paused entry would have sent. A
	// "continue" answer sends it without a handoff.
	EntryPrompt string `json:"entry_prompt"`
	// ExtractedHandoff holds the successful extraction across a reset failure,
	// so a retry reuses it instead of spending another extraction call.
	ExtractedHandoff string `json:"extracted_handoff,omitempty"`
}

// PendingContinuationRecord decodes the pending-continuation record from task
// metadata. It returns nil when the key is absent or malformed.
func PendingContinuationRecord(metadata map[string]interface{}) *PendingContinuation {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[MetaKeyPendingContinuation]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case *PendingContinuation:
		return v
	case PendingContinuation:
		return &v
	case map[string]interface{}:
		return pendingContinuationFromMap(v)
	}
	return nil
}

func pendingContinuationFromMap(m map[string]interface{}) *PendingContinuation {
	record := &PendingContinuation{}
	record.Stamp, _ = m["stamp"].(string)
	record.StepID, _ = m["step_id"].(string)
	record.EntryID, _ = m["entry_id"].(string)
	record.Case, _ = m["case"].(string)
	record.SourceSessionID, _ = m["source_session_id"].(string)
	record.Reason, _ = m["reason"].(string)
	record.EntryPrompt, _ = m["entry_prompt"].(string)
	record.ExtractedHandoff, _ = m["extracted_handoff"].(string)
	if record.Stamp == "" {
		return nil
	}
	return record
}
