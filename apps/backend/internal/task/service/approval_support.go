package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"go.uber.org/zap"
)

// ErrApprovalCommentsChanged reports that an approval answer referenced plan
// comments that are no longer pending (stale or foreign). The clarification
// layer maps it to an answer-validation error so the client can refresh.
var ErrApprovalCommentsChanged = errors.New("approval plan comments changed")

// ApprovalSupport implements the clarification approval subject reader and
// comment consumer on top of the task plan and document services.
type ApprovalSupport struct {
	plans  *PlanService
	docs   *DocumentService
	logger *logger.Logger
}

// NewApprovalSupport creates an ApprovalSupport.
func NewApprovalSupport(plans *PlanService, docs *DocumentService, log *logger.Logger) *ApprovalSupport {
	return &ApprovalSupport{
		plans:  plans,
		docs:   docs,
		logger: log.WithFields(zap.String("component", "approval-support")),
	}
}

// CurrentVersion returns the subject's current optimistic-concurrency token.
// A missing plan or document reports an empty version rather than an error, so
// the approval answer still resolves.
func (a *ApprovalSupport) CurrentVersion(ctx context.Context, taskID, subject, documentKey string) (string, error) {
	switch subject {
	case "task_plan":
		if a.plans == nil {
			return "", nil
		}
		plan, err := a.plans.GetPlan(ctx, taskID)
		if err != nil {
			if errors.Is(err, ErrTaskPlanNotFound) {
				return "", nil
			}
			return "", err
		}
		if plan == nil {
			return "", nil
		}
		return plan.WriteVersion, nil
	case "document":
		return a.documentVersion(ctx, taskID, documentKey)
	default:
		return "", nil
	}
}

func (a *ApprovalSupport) documentVersion(ctx context.Context, taskID, documentKey string) (string, error) {
	if a.docs == nil {
		return "", nil
	}
	revisions, err := a.docs.ListRevisions(ctx, taskID, documentKey, 1)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFound) {
			return "", nil
		}
		return "", err
	}
	if len(revisions) > 0 && revisions[0] != nil {
		return revisions[0].ID, nil
	}
	doc, err := a.docs.GetDocument(ctx, taskID, documentKey)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFound) {
			return "", nil
		}
		return "", err
	}
	if doc == nil {
		return "", nil
	}
	return doc.ID, nil
}

// RenderPlanComments validates refs against the task's current pending plan
// comments and returns the canonical Markdown block plus the resolved ids.
func (a *ApprovalSupport) RenderPlanComments(
	ctx context.Context,
	taskID string,
	refs []models.TaskPlanCommentRef,
) (string, []string, error) {
	if a.plans == nil {
		return "", nil, ErrApprovalCommentsChanged
	}
	snapshot, err := a.plans.ListPlanComments(ctx, taskID)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrApprovalCommentsChanged, err)
	}
	comments, ids, ok := resolveApprovalComments(snapshot, refs)
	if !ok {
		return "", nil, ErrApprovalCommentsChanged
	}
	return plancomments.FormatComments(comments), ids, nil
}

// ConsumePlanComments deletes the referenced pending comments through the
// plan-comment service, which publishes task_plan.comments.changed. It is
// best-effort at the clarification layer.
func (a *ApprovalSupport) ConsumePlanComments(
	ctx context.Context,
	taskID string,
	refs []models.TaskPlanCommentRef,
) error {
	if a.plans == nil {
		return ErrApprovalCommentsChanged
	}
	snapshot, err := a.plans.ListPlanComments(ctx, taskID)
	if err != nil {
		return err
	}
	byID := make(map[string]*models.TaskPlanComment, len(snapshot.Comments))
	for _, comment := range snapshot.Comments {
		byID[comment.ID] = comment
	}
	for _, ref := range refs {
		comment := byID[ref.ID]
		if comment == nil || comment.Version != ref.Version {
			continue
		}
		if _, err := a.plans.DeletePlanComment(ctx, DeletePlanCommentRequest{
			TaskID:          taskID,
			PlanID:          snapshot.PlanID,
			ID:              ref.ID,
			ExpectedVersion: ref.Version,
		}); err != nil {
			return fmt.Errorf("consume approval plan comment %s: %w", ref.ID, err)
		}
	}
	return nil
}

// resolveApprovalComments validates exact refs against the snapshot and
// returns the comments in (created_at, id) order plus their ids.
func resolveApprovalComments(
	snapshot *models.TaskPlanCommentSnapshot,
	refs []models.TaskPlanCommentRef,
) ([]*models.TaskPlanComment, []string, bool) {
	if snapshot == nil || len(refs) == 0 {
		return nil, nil, false
	}
	byID := make(map[string]*models.TaskPlanComment, len(snapshot.Comments))
	for _, comment := range snapshot.Comments {
		byID[comment.ID] = comment
	}
	seen := make(map[string]struct{}, len(refs))
	comments := make([]*models.TaskPlanComment, 0, len(refs))
	for _, ref := range refs {
		comment := byID[ref.ID]
		if _, duplicate := seen[ref.ID]; duplicate || comment == nil || comment.Version != ref.Version {
			return nil, nil, false
		}
		seen[ref.ID] = struct{}{}
		comments = append(comments, comment)
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].CreatedAt.Equal(comments[j].CreatedAt) {
			return comments[i].ID < comments[j].ID
		}
		return comments[i].CreatedAt.Before(comments[j].CreatedAt)
	})
	ids := make([]string, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.ID)
	}
	return comments, ids, true
}
