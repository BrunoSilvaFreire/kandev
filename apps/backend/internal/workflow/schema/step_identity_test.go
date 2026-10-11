package schema

import (
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
)

func TestEnsureStepIdentityBackfillsAndIsIdempotent(t *testing.T) {
	raw, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })
	_, err = raw.Exec(`CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, name TEXT NOT NULL)`)
	require.NoError(t, err)
	_, err = raw.Exec(`INSERT INTO workflow_steps (id, workflow_id, name) VALUES ('step-a', 'wf-a', 'A'), ('step-b', 'wf-a', 'B')`)
	require.NoError(t, err)

	for range 2 {
		require.NoError(t, EnsureStepIdentity(raw, db.NewRequiredMigrateLogger(raw, nil)))
	}
	var first, second string
	require.NoError(t, raw.Get(&first, `SELECT portable_key FROM workflow_steps WHERE id = 'step-a'`))
	require.NoError(t, raw.Get(&second, `SELECT portable_key FROM workflow_steps WHERE id = 'step-b'`))
	require.NotEmpty(t, first)
	require.NotEqual(t, first, second)
	var retiredAtExists int
	require.NoError(t, raw.Get(&retiredAtExists, `SELECT COUNT(*) FROM pragma_table_info('workflow_steps') WHERE name = 'retired_at'`))
	require.Equal(t, 1, retiredAtExists)
}
