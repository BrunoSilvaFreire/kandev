package clarification

import (
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func continuationRecoveryBundleMessages(meta *ContinuationRecoveryMeta) []*taskmodels.Message {
	return []*taskmodels.Message{{
		Type:     "clarification_request",
		Metadata: map[string]any{"continuation_recovery": meta},
	}}
}

func TestContinuationRecoveryQuestion_Shape(t *testing.T) {
	q := ContinuationRecoveryQuestion()
	assert.Equal(t, ContinuationRecoveryQuestionID, q.ID)
	require.Len(t, q.Options, 2)
	assert.Equal(t, ContinuationRecoveryDecisionRetry, q.Options[0].ID)
	assert.Equal(t, ContinuationRecoveryDecisionContinue, q.Options[1].ID)
}

func TestIsContinuationRecoveryBundle(t *testing.T) {
	require.True(t, IsContinuationRecoveryBundle(continuationRecoveryBundleMessages(&ContinuationRecoveryMeta{Stamp: "s"})))
	assert.False(t, IsContinuationRecoveryBundle(nil))
	assert.False(t, IsContinuationRecoveryBundle([]*taskmodels.Message{{Metadata: map[string]any{"approval": ApprovalMeta{Subject: ApprovalSubjectTaskPlan}}}}))
	assert.False(t, IsContinuationRecoveryBundle(continuationRecoveryBundleMessages(&ContinuationRecoveryMeta{})), "an unstamped meta is not a recovery bundle")
}

func TestValidateContinuationRecoveryBundle(t *testing.T) {
	q := ContinuationRecoveryQuestion()
	require.NoError(t, validateContinuationRecoveryBundle([]Question{q}, &ContinuationRecoveryMeta{Stamp: "s"}))
	require.Error(t, validateContinuationRecoveryBundle([]Question{q}, &ContinuationRecoveryMeta{}), "missing stamp")

	missing := q
	missing.Options = missing.Options[:1]
	require.Error(t, validateContinuationRecoveryBundle([]Question{missing}, &ContinuationRecoveryMeta{Stamp: "s"}), "missing option")

	wrongID := q
	wrongID.ID = "other"
	require.Error(t, validateContinuationRecoveryBundle([]Question{wrongID}, &ContinuationRecoveryMeta{Stamp: "s"}), "wrong question id")

	require.NoError(t, validateContinuationRecoveryBundle([]Question{q}, nil), "a normal bundle is untouched")
}

func TestContinuationRecoveryDecision(t *testing.T) {
	decision, err := continuationRecoveryDecision(Outcome{Answers: []Answer{{
		QuestionID:      ContinuationRecoveryQuestionID,
		SelectedOptions: []string{ContinuationRecoveryDecisionRetry},
	}}})
	require.NoError(t, err)
	assert.Equal(t, ContinuationRecoveryDecisionRetry, decision)

	_, err = continuationRecoveryDecision(Outcome{Answers: []Answer{{
		QuestionID:      ContinuationRecoveryQuestionID,
		SelectedOptions: []string{"bogus"},
	}}})
	require.Error(t, err)
}
