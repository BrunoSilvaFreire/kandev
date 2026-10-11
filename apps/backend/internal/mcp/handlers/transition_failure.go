package handlers

import (
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/transitionfailure"
)

// stepChangedFailure is the shared envelope for a completion signal sent from a
// turn that started in an earlier step.
func stepChangedFailure(launchStepID, currentStepID string) transitionfailure.Failure {
	failure := transitionfailure.Classify(repoerrors.ErrStepChanged, transitionfailure.Input{})
	failure.Message = "The workflow step changed before the completion signal was recorded."
	failure.Reasons[0].StepID = currentStepID
	failure.Reasons[0].Observed = "turn started in step " + launchStepID
	failure.Reasons[0].Required = "a turn started in step " + currentStepID
	failure.Reasons[0].Action = "End this turn and have the user resume the session for the current step, then signal completion from the new turn."
	failure.Reasons[0].CTA = nil
	failure.Remediation = failure.Reasons[0].Action
	return failure
}
