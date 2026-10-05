package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingContinuationRecord_DecodesMap(t *testing.T) {
	metadata := map[string]interface{}{
		MetaKeyPendingContinuation: map[string]interface{}{
			"stamp":             "s-1",
			"step_id":           "step-1",
			"entry_id":          "42",
			"case":              PendingContinuationCaseCold,
			"source_session_id": "sess-1",
			"reason":            PendingContinuationReasonResetFailed,
			"entry_prompt":      "do the thing",
			"extracted_handoff": "facts",
		},
	}
	record := PendingContinuationRecord(metadata)
	require.NotNil(t, record)
	assert.Equal(t, "s-1", record.Stamp)
	assert.Equal(t, "step-1", record.StepID)
	assert.Equal(t, "42", record.EntryID)
	assert.Equal(t, PendingContinuationCaseCold, record.Case)
	assert.Equal(t, "sess-1", record.SourceSessionID)
	assert.Equal(t, PendingContinuationReasonResetFailed, record.Reason)
	assert.Equal(t, "do the thing", record.EntryPrompt)
	assert.Equal(t, "facts", record.ExtractedHandoff)
}

func TestPendingContinuationRecord_MissingOrUnstampedIsNil(t *testing.T) {
	assert.Nil(t, PendingContinuationRecord(nil))
	assert.Nil(t, PendingContinuationRecord(map[string]interface{}{}))
	assert.Nil(t, PendingContinuationRecord(map[string]interface{}{
		MetaKeyPendingContinuation: map[string]interface{}{"reason": "x"},
	}), "a record without a stamp cannot be CAS-cleared, so it is ignored")
}

func TestPublicTaskMetadata_RedactsPendingContinuation(t *testing.T) {
	public := PublicTaskMetadata(map[string]interface{}{
		MetaKeyPendingContinuation: &PendingContinuation{Stamp: "s-1", EntryPrompt: "secret prompt"},
		"visible":                  "kept",
	})
	require.Contains(t, public, "visible")
	assert.NotContains(t, public, MetaKeyPendingContinuation)
}
