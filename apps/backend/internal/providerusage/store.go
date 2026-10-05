package providerusage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const observationColumns = `account_key, provider, window_label, kind,
	utilization_pct, weighted_tokens, model, reset_at, observed_at, source`

const insertObservationBase = `INSERT INTO provider_usage_observations (` +
	observationColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Repository owns the provider-usage tables.
type Repository struct {
	writer *sqlx.DB
	reader *sqlx.DB
	driver string

	insertDoNothing string
	insertTokens    string
}

// New creates the schema and returns a repository over the given handles.
func New(writer, reader *sqlx.DB) (*Repository, error) {
	r := &Repository{writer: writer, reader: reader, driver: writer.DriverName()}
	if err := r.initSchema(); err != nil {
		return nil, err
	}
	conflict := `(account_key, window_label, kind, source, observed_at, model)`
	r.insertDoNothing = writer.Rebind(insertObservationBase + ` ON CONFLICT ` + conflict + ` DO NOTHING`)
	r.insertTokens = writer.Rebind(insertObservationBase + ` ON CONFLICT ` + conflict +
		` DO UPDATE SET weighted_tokens = provider_usage_observations.weighted_tokens + excluded.weighted_tokens`)
	return r, nil
}

func (r *Repository) initSchema() error {
	idCol := "id INTEGER PRIMARY KEY AUTOINCREMENT"
	pctCol := "REAL"
	boolCol := "INTEGER NOT NULL DEFAULT 0"
	if dialect.IsPostgres(r.driver) {
		idCol = "id BIGSERIAL PRIMARY KEY"
		pctCol = "DOUBLE PRECISION"
		boolCol = "BOOLEAN NOT NULL DEFAULT false"
	}
	_, err := r.writer.Exec(`
	CREATE TABLE IF NOT EXISTS provider_usage_observations (
		` + idCol + `,
		account_key TEXT NOT NULL,
		provider TEXT NOT NULL,
		window_label TEXT NOT NULL,
		kind TEXT NOT NULL,
		utilization_pct ` + pctCol + `,
		weighted_tokens BIGINT,
		model TEXT NOT NULL DEFAULT '',
		reset_at TIMESTAMP,
		observed_at TIMESTAMP NOT NULL,
		source TEXT NOT NULL,
		UNIQUE(account_key, window_label, kind, source, observed_at, model)
	);
	CREATE INDEX IF NOT EXISTS idx_provider_usage_observations_account
		ON provider_usage_observations(account_key, observed_at);
	CREATE TABLE IF NOT EXISTS provider_usage_index_files (
		path_hash TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		mtime TIMESTAMP,
		byte_offset BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE TABLE IF NOT EXISTS provider_usage_sources (
		source TEXT PRIMARY KEY,
		enabled ` + boolCol + `,
		changed_at TIMESTAMP NOT NULL
	);
	`)
	if err != nil {
		return fmt.Errorf("init provider usage schema: %w", err)
	}
	return nil
}

// InsertObservations writes a batch idempotently. Token rows add to an
// existing bucket; other kinds are ignored on conflict.
func (r *Repository) InsertObservations(ctx context.Context, observations []Observation) error {
	if len(observations) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i := range observations {
		obs := &observations[i]
		stmt := r.insertDoNothing
		if obs.Kind == KindTokens {
			stmt = r.insertTokens
		}
		if _, err := tx.ExecContext(ctx, stmt,
			obs.AccountKey, obs.Provider, obs.WindowLabel, obs.Kind,
			obs.UtilizationPct, obs.WeightedTokens, obs.Model,
			nullTime(obs.ResetAt), obs.ObservedAt, obs.Source,
		); err != nil {
			return fmt.Errorf("insert observation: %w", err)
		}
	}
	return tx.Commit()
}

// ListObservations returns one account's observations at or after since,
// oldest first.
func (r *Repository) ListObservations(ctx context.Context, accountKey string, since time.Time) ([]Observation, error) {
	query := r.reader.Rebind(`SELECT ` + observationColumns + `
		FROM provider_usage_observations
		WHERE account_key = ? AND observed_at >= ?
		ORDER BY observed_at`)
	rows, err := r.reader.QueryContext(ctx, query, accountKey, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Observation
	for rows.Next() {
		var (
			obs       Observation
			windowPct sql.NullFloat64
			tokens    sql.NullInt64
			resetAt   sql.NullTime
		)
		if err := rows.Scan(&obs.AccountKey, &obs.Provider, &obs.WindowLabel, &obs.Kind,
			&windowPct, &tokens, &obs.Model, &resetAt, &obs.ObservedAt, &obs.Source); err != nil {
			return nil, err
		}
		if windowPct.Valid {
			v := windowPct.Float64
			obs.UtilizationPct = &v
		}
		if tokens.Valid {
			v := tokens.Int64
			obs.WeightedTokens = &v
		}
		if resetAt.Valid {
			obs.ResetAt = resetAt.Time
		}
		out = append(out, obs)
	}
	return out, rows.Err()
}

// LatestLimitHit returns the most recently recorded provider limit hit for an
// account. A missing observation is not an error.
func (r *Repository) LatestLimitHit(ctx context.Context, accountKey string) (observedAt, resetAt time.Time, ok bool, err error) {
	query := r.reader.Rebind(`SELECT observed_at, reset_at
		FROM provider_usage_observations
		WHERE account_key = ? AND kind = ?
		ORDER BY observed_at DESC LIMIT 1`)
	var reset sql.NullTime
	err = r.reader.QueryRowContext(ctx, query, accountKey, KindLimitHit).Scan(&observedAt, &reset)
	if err == sql.ErrNoRows {
		return time.Time{}, time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	if reset.Valid {
		resetAt = reset.Time
	}
	return observedAt, resetAt, true, nil
}

// Prune deletes observations older than before.
func (r *Repository) Prune(ctx context.Context, before time.Time) error {
	_, err := r.writer.ExecContext(ctx,
		r.writer.Rebind(`DELETE FROM provider_usage_observations WHERE observed_at < ?`), before)
	return err
}

// GetCursor returns the stored cursor for a file key.
func (r *Repository) GetCursor(ctx context.Context, pathHash string) (FileCursor, bool, error) {
	query := r.reader.Rebind(`SELECT path_hash, source, size, mtime, byte_offset, updated_at
		FROM provider_usage_index_files WHERE path_hash = ?`)
	var (
		cursor  FileCursor
		mtime   sql.NullTime
		updated sql.NullTime
	)
	err := r.reader.QueryRowxContext(ctx, query, pathHash).Scan(
		&cursor.PathHash, &cursor.Source, &cursor.Size, &mtime, &cursor.ByteOffset, &updated)
	if err == sql.ErrNoRows {
		return FileCursor{}, false, nil
	}
	if err != nil {
		return FileCursor{}, false, err
	}
	if mtime.Valid {
		cursor.Mtime = mtime.Time
	}
	if updated.Valid {
		cursor.UpdatedAt = updated.Time
	}
	return cursor, true, nil
}

// PutCursor upserts a file cursor.
func (r *Repository) PutCursor(ctx context.Context, cursor FileCursor) error {
	if cursor.UpdatedAt.IsZero() {
		cursor.UpdatedAt = time.Now().UTC()
	}
	query := r.writer.Rebind(`INSERT INTO provider_usage_index_files
		(path_hash, source, size, mtime, byte_offset, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (path_hash) DO UPDATE SET
			source = excluded.source,
			size = excluded.size,
			mtime = excluded.mtime,
			byte_offset = excluded.byte_offset,
			updated_at = excluded.updated_at`)
	_, err := r.writer.ExecContext(ctx, query,
		cursor.PathHash, cursor.Source, cursor.Size, nullTime(cursor.Mtime), cursor.ByteOffset, cursor.UpdatedAt)
	return err
}

// ListSources returns every persisted source state. A missing row is off.
func (r *Repository) ListSources(ctx context.Context) (map[string]Source, error) {
	rows, err := r.reader.QueryContext(ctx, `SELECT source, enabled, changed_at FROM provider_usage_sources`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	sources := make(map[string]Source)
	for rows.Next() {
		var (
			src     Source
			enabled any
			changed sql.NullTime
		)
		if err := rows.Scan(&src.Source, &enabled, &changed); err != nil {
			return nil, err
		}
		src.Enabled = enabledValue(enabled)
		if changed.Valid {
			src.ChangedAt = changed.Time
		}
		sources[src.Source] = src
	}
	return sources, rows.Err()
}

// SetSource enables or disables one source.
func (r *Repository) SetSource(ctx context.Context, source string, enabled bool) error {
	query := r.writer.Rebind(`INSERT INTO provider_usage_sources (source, enabled, changed_at)
		VALUES (?, ?, ?)
		ON CONFLICT (source) DO UPDATE SET enabled = excluded.enabled, changed_at = excluded.changed_at`)
	_, err := r.writer.ExecContext(ctx, query, source, enabled, time.Now().UTC())
	return err
}

// DeleteSource removes a source's observations and cursors in one transaction.
func (r *Repository) DeleteSource(ctx context.Context, source string) error {
	tx, err := r.writer.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, r.writer.Rebind(
		`DELETE FROM provider_usage_observations WHERE source = ?`), source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.writer.Rebind(
		`DELETE FROM provider_usage_index_files WHERE source = ?`), source); err != nil {
		return err
	}
	return tx.Commit()
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func enabledValue(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case int64:
		return typed != 0
	case int:
		return typed != 0
	case []byte:
		return string(typed) == "1" || string(typed) == "true"
	case string:
		return typed == "1" || typed == "true"
	default:
		return false
	}
}
