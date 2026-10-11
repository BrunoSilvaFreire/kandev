package service

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// QuickChatActivitySourceMessage means the newest message set the time.
	QuickChatActivitySourceMessage = "message"
	// QuickChatActivitySourceCreated means the chat has no messages yet.
	QuickChatActivitySourceCreated = "created"

	quickChatTitleFallbackRunes = 80
	quickChatBrowseTaskLimit    = 5
)

// QuickChatBrowse is the conversation-level detail the browse page shows for
// one Quick Chat. Every field is evidence from the chat's own rows.
type QuickChatBrowse struct {
	LastActivityAt time.Time        `json:"last_activity_at"`
	ActivitySource string           `json:"activity_source"`
	TitleFallback  string           `json:"title_fallback,omitempty"`
	RepositoryIDs  []string         `json:"repository_ids,omitempty"`
	Tasks          []models.TaskRef `json:"tasks,omitempty"`
	TaskCount      int              `json:"task_count,omitempty"`
}

type quickChatBrowseReader interface {
	GetLastConversationTimeBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]time.Time, error)
	GetFirstUserMessageBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]string, error)
	ListChildTaskRefsByParentIDs(ctx context.Context, parentIDs []string) (map[string][]models.TaskRef, error)
}

// QuickChatBrowseDetails returns browse detail keyed by session ID for items
// already returned by ListQuickChatSessions, which authorized the workspace.
func (s *Service) QuickChatBrowseDetails(ctx context.Context, items []QuickChatSession) (map[string]QuickChatBrowse, error) {
	details := make(map[string]QuickChatBrowse, len(items))
	if len(items) == 0 {
		return details, nil
	}
	sessionIDs := make([]string, 0, len(items))
	taskIDs := make([]string, 0, len(items))
	for _, item := range items {
		sessionIDs = append(sessionIDs, item.SessionID)
		taskIDs = append(taskIDs, item.TaskID)
	}
	repositories, err := s.taskRepos.ListTaskRepositoriesByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	lastMessage := map[string]time.Time{}
	firstMessage := map[string]string{}
	children := map[string][]models.TaskRef{}
	if reader, ok := s.tasks.(quickChatBrowseReader); ok {
		if lastMessage, err = reader.GetLastConversationTimeBySessionIDs(ctx, sessionIDs); err != nil {
			return nil, err
		}
		if firstMessage, err = reader.GetFirstUserMessageBySessionIDs(ctx, sessionIDs); err != nil {
			return nil, err
		}
		if children, err = reader.ListChildTaskRefsByParentIDs(ctx, taskIDs); err != nil {
			return nil, err
		}
	}
	for _, item := range items {
		detail := QuickChatBrowse{LastActivityAt: item.createdAt.UTC(), ActivitySource: QuickChatActivitySourceCreated}
		if at, ok := lastMessage[item.SessionID]; ok && !at.IsZero() {
			detail.LastActivityAt, detail.ActivitySource = at.UTC(), QuickChatActivitySourceMessage
		}
		if item.Name == "" {
			detail.TitleFallback = quickChatTitleExcerpt(firstMessage[item.SessionID])
		}
		for _, repository := range repositories[item.TaskID] {
			detail.RepositoryIDs = append(detail.RepositoryIDs, repository.RepositoryID)
		}
		tasks := children[item.TaskID]
		detail.TaskCount = len(tasks)
		if len(tasks) > quickChatBrowseTaskLimit {
			tasks = tasks[:quickChatBrowseTaskLimit]
		}
		detail.Tasks = tasks
		details[item.SessionID] = detail
	}
	return details, nil
}

// quickChatTitleExcerpt collapses whitespace and cuts on a rune boundary.
func quickChatTitleExcerpt(content string) string {
	text := strings.Join(strings.Fields(content), " ")
	runes := []rune(text)
	if len(runes) <= quickChatTitleFallbackRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:quickChatTitleFallbackRunes])) + "…"
}
