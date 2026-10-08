package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	clarificationStatusAnswered = "answered"
	clarificationStatusPending  = "pending"
	clarificationStatusRejected = "rejected"
	// clarificationStatusResponding is an intra-transaction claim marker.
	// CompleteActiveClarificationBundle never commits it independently.
	clarificationStatusResponding = "responding"

	clarificationResponseDeliveryPendingKey = "response_delivery_pending"
)

func nonTerminalSessionPredicate(messageAlias string) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM task_sessions session_row
		WHERE session_row.id = %s.task_session_id
		  AND session_row.state NOT IN ('%s', '%s', '%s')
	)`,
		messageAlias,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	)
}

// DetachActiveClarificationMessagesBySessionID atomically marks only pending,
// current-turn clarification rows as detached and returns the rows that changed.
func (r *Repository) DetachActiveClarificationMessagesBySessionID(
	ctx context.Context,
	sessionID string,
) ([]*models.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clarification detach: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	if err := lockSessionTurnWrites(ctx, tx, drv, sessionID); err != nil {
		return nil, err
	}
	updatedAt := r.nowUTC()
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_session_id = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s
		  AND turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = task_session_messages.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at
	`, clarificationDetachedMetadataExpr(drv),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "status"),
		clarificationNotDetachedPredicate(drv),
		predicate, orderBy)
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), updatedAt, sessionID)
	if err != nil {
		return nil, fmt.Errorf("detach active clarification messages: %w", err)
	}
	messages, _, err := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan detached clarification messages: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close detached clarification rows: %w", closeErr)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clarification detach: %w", err)
	}
	return messages, nil
}

// ExpireActiveClarificationBundle atomically expires one exact pending bundle
// only while it belongs to the session's current durable turn. The status and
// pending-ID predicates are evaluated by the UPDATE, so a stale expiry can
// never overwrite a concurrent answer or a newer bundle.
func (r *Repository) ExpireActiveClarificationBundle(
	ctx context.Context,
	sessionID, pendingID string,
) ([]*models.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clarification expiry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	if err := lockSessionTurnWrites(ctx, tx, drv, sessionID); err != nil {
		return nil, err
	}
	updatedAt := r.nowUTC()
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_session_id = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s = ?
		  AND turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = task_session_messages.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at
	`, clarificationExpiredMetadataExpr(drv),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "status"),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "pending_id"),
		predicate, orderBy)
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), updatedAt, sessionID, pendingID)
	if err != nil {
		return nil, fmt.Errorf("expire active clarification messages: %w", err)
	}
	messages, _, err := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan expired clarification messages: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close expired clarification rows: %w", closeErr)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clarification expiry: %w", err)
	}
	return messages, nil
}

func clarificationDetachedMetadataExpr(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return "jsonb_set(metadata::jsonb, '{agent_disconnected}', 'true'::jsonb)::text"
	}
	return "json_set(metadata, '$.agent_disconnected', json('true'))"
}

func clarificationExpiredMetadataExpr(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return "jsonb_set(jsonb_set(metadata::jsonb, '{agent_disconnected}', 'true'::jsonb), " +
			"'{status}', '\"expired\"'::jsonb)::text"
	}
	return "json_set(metadata, '$.agent_disconnected', json('true'), '$.status', 'expired')"
}

func clarificationNotDetachedPredicate(driverName string) string {
	value := dialect.JSONExtract(driverName, "task_session_messages.metadata", "agent_disconnected")
	return fmt.Sprintf("CAST(COALESCE(%s, '') AS TEXT) NOT IN ('true', '1')", value)
}

func clarificationResponseDeliveryPendingPredicate(driverName, messageAlias string) string {
	value := dialect.JSONExtract(
		driverName,
		messageAlias+".metadata",
		clarificationResponseDeliveryPendingKey,
	)
	return fmt.Sprintf("CAST(COALESCE(%s, '') AS TEXT) IN ('true', '1')", value)
}

// CompleteActiveClarificationBundle atomically claims a current-turn pending
// bundle and persists its terminal state. Exactly one concurrent responder can
// transition the rows; superseded or already-terminal bundles return claimed=false.
func (r *Repository) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*models.Message, bool, error) {
	if status != clarificationStatusAnswered && status != clarificationStatusRejected {
		return nil, false, fmt.Errorf("invalid clarification terminal status %q", status)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	drv := r.db.DriverName()
	if err := r.lockClarificationBundleTurnWrites(ctx, tx, drv, pendingID); err != nil {
		return nil, false, err
	}
	claimedRows, err := r.claimActiveClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	if claimedRows == 0 {
		return nil, false, nil
	}
	messages, err := r.loadClaimedClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	if int64(len(messages)) != claimedRows {
		return nil, false, fmt.Errorf("claimed %d clarification rows but loaded %d", claimedRows, len(messages))
	}
	if err := r.completeClaimedClarificationMessages(ctx, tx, drv, messages, status, responses); err != nil {
		return nil, false, err
	}
	if err := r.persistPlanApprovalReceiptsInTx(ctx, tx, drv, messages, status, responses); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit clarification bundle completion: %w", err)
	}
	return messages, true, nil
}

// RestoreActiveClarificationBundle reopens a current-turn terminal bundle when
// its detached resume event could not be published and returns the committed
// pending rows. The status check makes the rollback idempotent and prevents an
// older turn from becoming active again.
func (r *Repository) RestoreActiveClarificationBundle(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	if terminalStatus != clarificationStatusAnswered && terminalStatus != clarificationStatusRejected {
		return nil, false, fmt.Errorf("invalid clarification terminal status %q", terminalStatus)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	claimIDs, err := clarificationClaimIDs(claimedMessages)
	if err != nil {
		return nil, false, err
	}
	drv := r.db.DriverName()
	if err := r.lockClarificationBundleTurnWrites(ctx, tx, drv, pendingID); err != nil {
		return nil, false, err
	}
	messages, err := r.loadRestorableClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	restorable := make([]*models.Message, 0, len(claimIDs))
	for _, message := range messages {
		if _, claimed := claimIDs[message.ID]; !claimed {
			continue
		}
		if status, _ := message.Metadata["status"].(string); status != terminalStatus {
			return nil, false, nil
		}
		restorable = append(restorable, message)
	}
	if len(restorable) != len(claimIDs) {
		return nil, false, nil
	}
	if err := r.restoreClarificationMessages(
		ctx,
		tx,
		drv,
		restorable,
		terminalStatus,
	); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit clarification bundle restore: %w", err)
	}
	return restorable, true, nil
}

func (r *Repository) lockClarificationBundleTurnWrites(
	ctx context.Context,
	tx *sqlx.Tx,
	driverName, pendingID string,
) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	pendingIDExpr := dialect.JSONExtract(driverName, "metadata", "pending_id")
	query := fmt.Sprintf(`
		SELECT DISTINCT task_session_id
		FROM task_session_messages
		WHERE type = 'clarification_request'
		  AND %s = ?
	`, pendingIDExpr)
	var sessionIDs []string
	if err := tx.SelectContext(ctx, &sessionIDs, r.db.Rebind(query), pendingID); err != nil {
		return fmt.Errorf("load clarification bundle sessions: %w", err)
	}
	sort.Strings(sessionIDs)
	if err := lockSessionTurnWrites(ctx, tx, driverName, sessionIDs...); err != nil {
		return err
	}
	// Terminal session writers acquire row locks through UPDATE. Locking the
	// same rows before evaluating the non-terminal claim predicate forces one
	// side to observe the other's committed state.
	for _, sessionID := range sessionIDs {
		var lockedSessionID string
		if err := tx.GetContext(
			ctx,
			&lockedSessionID,
			r.db.Rebind(`SELECT id FROM task_sessions WHERE id = ? FOR UPDATE`),
			sessionID,
		); err != nil {
			return fmt.Errorf("lock clarification session %q: %w", sessionID, err)
		}
	}
	return nil
}

func clarificationClaimIDs(messages []*models.Message) (map[string]struct{}, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("clarification restore requires claimed messages")
	}
	ids := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message == nil || message.ID == "" {
			return nil, fmt.Errorf("clarification restore received an empty message id")
		}
		if _, duplicate := ids[message.ID]; duplicate {
			return nil, fmt.Errorf("clarification restore received duplicate message %s", message.ID)
		}
		ids[message.ID] = struct{}{}
	}
	return ids, nil
}

func (r *Repository) loadRestorableClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) ([]*models.Message, error) {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	bundlePendingIDExpr := dialect.JSONExtract(drv, "bundle.metadata", "pending_id")
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	// A pending ID spanning message types, sessions, or turns is malformed. The
	// NOT EXISTS guard intentionally makes the whole bundle ineligible to restore.
	query := fmt.Sprintf(`
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at
		FROM task_session_messages m
		WHERE %s = ?
		  AND m.type = 'clarification_request'
		  AND %s
		  AND %s
		  AND m.turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = m.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM task_session_messages bundle
			WHERE %s = ?
			  AND (
				bundle.type != 'clarification_request'
				OR bundle.task_session_id != m.task_session_id
				OR bundle.turn_id != m.turn_id
			  )
		  )
		ORDER BY m.created_at ASC, m.id ASC
	`, pendingIDExpr,
		nonTerminalSessionPredicate("m"),
		clarificationResponseDeliveryPendingPredicate(drv, "m"),
		predicate, orderBy,
		bundlePendingIDExpr,
	)
	rows, err := tx.QueryxContext(
		ctx,
		r.db.Rebind(query),
		pendingID,
		pendingID,
	)
	if err != nil {
		return nil, fmt.Errorf("load restorable clarification bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages, _, scanErr := scanMessageRows(rows, 0)
	if scanErr != nil {
		return nil, fmt.Errorf("scan restorable clarification bundle: %w", scanErr)
	}
	return messages, nil
}

func (r *Repository) restoreClarificationMessages(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	terminalStatus string,
) error {
	updatedAt := r.nowUTC()
	statusExpr := dialect.JSONExtract(drv, "metadata", "status")
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	updateQuery := r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = ?, updated_at = ?
		WHERE id = ? AND %s = ?
		  AND %s
		  AND %s
		  AND turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = task_session_messages.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
	`,
		statusExpr,
		clarificationResponseDeliveryPendingPredicate(drv, "task_session_messages"),
		nonTerminalSessionPredicate("task_session_messages"),
		predicate, orderBy,
	))
	restoredMetadataByMessage := make([]map[string]interface{}, len(messages))
	for i, message := range messages {
		restoredMetadata := maps.Clone(message.Metadata)
		restoredMetadata["status"] = clarificationStatusPending
		delete(restoredMetadata, "response")
		delete(restoredMetadata, clarificationResponseDeliveryPendingKey)
		metadataJSON, marshalErr := json.Marshal(restoredMetadata)
		if marshalErr != nil {
			return fmt.Errorf("marshal clarification message %s for restore: %w", message.ID, marshalErr)
		}
		result, updateErr := tx.ExecContext(
			ctx,
			updateQuery,
			string(metadataJSON),
			updatedAt,
			message.ID,
			terminalStatus,
		)
		if updateErr != nil {
			return fmt.Errorf("restore clarification message %s: %w", message.ID, updateErr)
		}
		updatedRows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("count restored clarification message %s: %w", message.ID, rowsErr)
		}
		if updatedRows != 1 {
			return fmt.Errorf("clarification message %s lost its terminal claim", message.ID)
		}
		restoredMetadataByMessage[i] = restoredMetadata
	}
	for i, message := range messages {
		message.Metadata = restoredMetadataByMessage[i]
		message.UpdatedAt = updatedAt
	}
	return nil
}

func (r *Repository) claimActiveClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) (int64, error) {
	// A pending ID spanning message types, sessions, or turns is malformed. The
	// NOT EXISTS guard intentionally makes the whole bundle ineligible to claim.
	// Detached rows are intentionally eligible: agent_disconnected means the
	// live waiter is gone, not that the current-turn question was resolved.
	// The responding marker cannot be stranded by a later terminal transition;
	// it is replaced with the requested terminal status before this transaction
	// commits or is rolled back with the transaction.
	claimAt := r.nowUTC()
	claimQuery := clarificationClaimQuery(drv)
	// pendingID is bound once for the outer bundle and once for the malformed-
	// bundle NOT EXISTS guard.
	result, err := tx.ExecContext(ctx, r.db.Rebind(claimQuery), claimAt, pendingID, pendingID)
	if err != nil {
		return 0, fmt.Errorf("claim active clarification bundle: %w", err)
	}
	claimedRows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count claimed clarification rows: %w", err)
	}
	return claimedRows, nil
}

// clarificationClaimQuery builds the shared claim statement for execution and
// the query-plan regression test. Both paths must use the same predicates.
func clarificationClaimQuery(driverName string) string {
	pendingIDExpr := dialect.JSONExtract(driverName, "task_session_messages.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(driverName, "task_session_messages.metadata", "status")
	bundlePendingIDExpr := dialect.JSONExtract(driverName, "bundle.metadata", "pending_id")
	predicate, orderBy := currentTurnAuthority(driverName, "turn_row")
	return fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE %s = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s
		  AND turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = task_session_messages.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM task_session_messages bundle
			WHERE %s = ?
			  AND (
				bundle.type != 'clarification_request'
				OR bundle.task_session_id != task_session_messages.task_session_id
				OR bundle.turn_id != task_session_messages.turn_id
			  )
		  )
	`, dialect.JSONSet(driverName, "metadata", "status", clarificationStatusResponding), pendingIDExpr, statusExpr,
		nonTerminalSessionPredicate("task_session_messages"),
		predicate, orderBy, bundlePendingIDExpr)
}

func (r *Repository) loadClaimedClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) ([]*models.Message, error) {
	claimedStatusExpr := dialect.JSONExtract(drv, "m.metadata", "status")
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(fmt.Sprintf(`
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at
		FROM task_session_messages m
		WHERE %s = ? AND %s = ?
		  AND m.turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = m.task_session_id
			  AND %s
			ORDER BY %s
			LIMIT 1
		  )
		ORDER BY m.created_at ASC, m.id ASC
	`, dialect.JSONExtract(drv, "m.metadata", "pending_id"), claimedStatusExpr,
		predicate, orderBy)), pendingID, clarificationStatusResponding)
	if err != nil {
		return nil, fmt.Errorf("load claimed clarification bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages, _, scanErr := scanMessageRows(rows, 0)
	if scanErr != nil {
		return nil, fmt.Errorf("scan claimed clarification bundle: %w", scanErr)
	}
	return messages, nil
}

func (r *Repository) completeClaimedClarificationMessages(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	status string,
	responses map[string]interface{},
) error {
	updatedAt := r.nowUTC()
	claimedStatusExpr := dialect.JSONExtract(drv, "metadata", "status")
	updateQuery := r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = ?, updated_at = ?
		WHERE id = ? AND %s = ?
	`, claimedStatusExpr))
	completedMetadataByMessage := make([]map[string]interface{}, len(messages))
	for i, message := range messages {
		questionID, _ := message.Metadata["question_id"].(string)
		if questionID == "" {
			return fmt.Errorf("clarification message %s is missing question_id", message.ID)
		}
		completedMetadata := maps.Clone(message.Metadata)
		completedMetadata["status"] = status
		completedMetadata[clarificationResponseDeliveryPendingKey] = true
		if status == clarificationStatusAnswered {
			response, ok := responses[questionID]
			if !ok {
				return fmt.Errorf("missing response for clarification question %s", questionID)
			}
			completedMetadata["response"] = response
		}
		metadataJSON, marshalErr := json.Marshal(completedMetadata)
		if marshalErr != nil {
			return fmt.Errorf("marshal clarification message %s: %w", message.ID, marshalErr)
		}
		updateResult, updateErr := tx.ExecContext(
			ctx,
			updateQuery,
			string(metadataJSON), updatedAt, message.ID, clarificationStatusResponding,
		)
		if updateErr != nil {
			return fmt.Errorf("complete clarification message %s: %w", message.ID, updateErr)
		}
		updatedRows, rowsErr := updateResult.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("count completed clarification message %s: %w", message.ID, rowsErr)
		}
		if updatedRows != 1 {
			return fmt.Errorf("clarification message %s lost its claim", message.ID)
		}
		completedMetadataByMessage[i] = completedMetadata
	}
	for i, message := range messages {
		message.Metadata = completedMetadataByMessage[i]
		message.UpdatedAt = updatedAt
	}
	return nil
}

//nolint:cyclop,funlen,gocognit // The transaction persists one receipt per winning approval answer in a single pass.
func (r *Repository) persistPlanApprovalReceiptsInTx(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	status string,
	responses map[string]interface{},
) error {
	if status != clarificationStatusAnswered || len(messages) == 0 {
		return nil
	}
	now := r.nowUTC()
	for _, message := range messages {
		if message == nil || message.Type != "clarification_request" || message.Metadata == nil {
			continue
		}
		rawApproval, ok := message.Metadata["approval"]
		if !ok || rawApproval == nil {
			continue
		}
		var subject, versionAtRequest, planRevisionID string
		switch a := rawApproval.(type) {
		case map[string]interface{}:
			subject, _ = a["subject"].(string)
			versionAtRequest, _ = a["version_at_request"].(string)
			planRevisionID, _ = a["plan_revision_id"].(string)
		default:
			data, err := json.Marshal(rawApproval)
			if err == nil {
				var m map[string]interface{}
				if err := json.Unmarshal(data, &m); err == nil {
					subject, _ = m["subject"].(string)
					versionAtRequest, _ = m["version_at_request"].(string)
					planRevisionID, _ = m["plan_revision_id"].(string)
				}
			}
		}
		if subject != "task_plan" || versionAtRequest == "" {
			continue
		}
		questionID, _ := message.Metadata["question_id"].(string)
		resp, hasResp := responses[questionID]
		if !hasResp {
			continue
		}
		decision, subjectEdited := extractApprovalResponse(resp)
		if decision != "approve" || subjectEdited {
			continue
		}
		if planRevisionID == "" {
			_ = tx.QueryRowContext(ctx, r.db.Rebind(`
				SELECT id FROM task_plan_revisions
				WHERE task_id = ? AND write_version = ?
				ORDER BY revision_number DESC LIMIT 1
			`), message.TaskID, versionAtRequest).Scan(&planRevisionID)
		}
		if planRevisionID == "" {
			continue
		}
		var planWriteVersion string
		err := tx.QueryRowContext(ctx, r.db.Rebind(`
			SELECT write_version FROM task_plans WHERE task_id = ?
		`), message.TaskID).Scan(&planWriteVersion)
		if err != nil || planWriteVersion != versionAtRequest {
			continue
		}
		var revWriteVersion string
		err = tx.QueryRowContext(ctx, r.db.Rebind(`
			SELECT write_version FROM task_plan_revisions WHERE id = ? AND task_id = ?
		`), planRevisionID, message.TaskID).Scan(&revWriteVersion)
		if err != nil || revWriteVersion != versionAtRequest {
			continue
		}
		receiptID := message.ID
		if receiptID == "" {
			receiptID = uuid.NewString()
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_plan_approval_receipts
				(id, task_id, plan_revision_id, write_version, decision, subject_edited, created_at)
			VALUES (?, ?, ?, ?, ?, 0, ?)
			ON CONFLICT(id) DO NOTHING
		`), receiptID, message.TaskID, planRevisionID, versionAtRequest, decision, now)
		if err != nil {
			return fmt.Errorf("persist plan approval receipt: %w", err)
		}
	}
	return nil
}

func extractApprovalResponse(resp interface{}) (decision string, subjectEdited bool) {
	if resp == nil {
		return "", false
	}
	switch v := resp.(type) {
	case map[string]interface{}:
		if appr, ok := v["approval"].(map[string]interface{}); ok {
			if d, ok := appr["decision"].(string); ok {
				decision = d
			}
			if se, ok := appr["subject_edited"].(bool); ok {
				subjectEdited = se
			}
		}
		if decision == "" {
			if sel, ok := v["selected_options"].([]string); ok && len(sel) > 0 {
				decision = sel[0]
			} else if sel, ok := v["selected_options"].([]interface{}); ok && len(sel) > 0 {
				if d, ok := sel[0].(string); ok {
					decision = d
				}
			}
		}
	default:
		data, err := json.Marshal(resp)
		if err == nil {
			var m map[string]interface{}
			if err := json.Unmarshal(data, &m); err == nil {
				return extractApprovalResponse(m)
			}
		}
	}
	return decision, subjectEdited
}
