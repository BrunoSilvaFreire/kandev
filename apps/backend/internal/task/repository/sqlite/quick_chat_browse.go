package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// quickChatFirstMessageScanChars bounds how much of a first message is read
// before the caller trims it to an excerpt.
const quickChatFirstMessageScanChars = 400

// GetFirstUserMessageBySessionIDs returns the leading text of each session's
// oldest user message. Sessions without one are absent from the result.
func (r *Repository) GetFirstUserMessageBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]string, error) {
	result := make(map[string]string, len(sessionIDs))
	for _, chunk := range chunkIDs(sessionIDs, sqliteMaxHostParams) {
		placeholders, ids := buildInPlaceholders(chunk)
		query := fmt.Sprintf(`
			SELECT m.task_session_id, SUBSTR(m.content, 1, %d)
			FROM task_session_messages m
			JOIN (
				SELECT task_session_id, MIN(created_at) AS first_at
				FROM task_session_messages
				WHERE author_type = 'user' AND type = 'message' AND task_session_id IN (%s)
				GROUP BY task_session_id
			) f ON f.task_session_id = m.task_session_id AND m.created_at = f.first_at
			WHERE m.author_type = 'user' AND m.type = 'message'
			ORDER BY m.task_session_id, m.id`, quickChatFirstMessageScanChars, placeholders)
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), ids...)
		if err != nil {
			return nil, fmt.Errorf("load first user message by session: %w", err)
		}
		for rows.Next() {
			var sessionID, content string
			if err := rows.Scan(&sessionID, &content); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan first user message by session: %w", err)
			}
			// Two messages can share the first timestamp; the lowest ID wins.
			if _, seen := result[sessionID]; !seen {
				result[sessionID] = content
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ListChildTaskRefsByParentIDs returns the unarchived direct children of each
// parent, oldest first.
func (r *Repository) ListChildTaskRefsByParentIDs(ctx context.Context, parentIDs []string) (map[string][]models.TaskRef, error) {
	result := make(map[string][]models.TaskRef, len(parentIDs))
	for _, chunk := range chunkIDs(parentIDs, sqliteMaxHostParams) {
		placeholders, ids := buildInPlaceholders(chunk)
		query := `
			SELECT parent_id, id, title
			FROM tasks
			WHERE parent_id IN (` + placeholders + `) AND archived_at IS NULL
			ORDER BY parent_id, created_at, id`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), ids...)
		if err != nil {
			return nil, fmt.Errorf("load child tasks by parent: %w", err)
		}
		for rows.Next() {
			var parentID string
			var ref models.TaskRef
			if err := rows.Scan(&parentID, &ref.ID, &ref.Title); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan child task by parent: %w", err)
			}
			result[parentID] = append(result[parentID], ref)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetLastConversationTimeBySessionIDs returns when each session's conversation
// last changed: the newest user or agent message, tool call, or request.
// Lifecycle rows a start, restart, or recovery writes (status, log, progress,
// error, script execution) are not conversation and never move the time.
// Sessions with no conversation row are absent from the result.
func (r *Repository) GetLastConversationTimeBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]time.Time, error) {
	result := make(map[string]time.Time, len(sessionIDs))
	for _, chunk := range chunkIDs(sessionIDs, sqliteMaxHostParams) {
		placeholders, ids := buildInPlaceholders(chunk)
		query := `
			SELECT task_session_id, MAX(updated_at)
			FROM task_session_messages
			WHERE task_session_id IN (` + placeholders + `)
				AND type NOT IN ('status', 'log', 'progress', 'error', 'script_execution')
			GROUP BY task_session_id`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), ids...)
		if err != nil {
			return nil, fmt.Errorf("load last conversation time by session: %w", err)
		}
		for rows.Next() {
			var sessionID string
			var raw interface{}
			if err := rows.Scan(&sessionID, &raw); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan last conversation time by session: %w", err)
			}
			at, err := parseTaskActivityTime(raw)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("parse last conversation time for session %q: %w", sessionID, err)
			}
			result[sessionID] = at
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}
