//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/service"
	dbmigrations "github.com/MACOS-DO/sub4api/migrations"
	"github.com/stretchr/testify/require"
)

func TestWSProbeMigrationPartitionsAndOldRows(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	schema := fmt.Sprintf("ws_probe_%d", time.Now().UnixNano())
	_, err := tx.ExecContext(ctx, "CREATE SCHEMA "+schema+"; SET LOCAL search_path TO "+schema)
	require.NoError(t, err)
	for _, name := range []string{"239_codex_ticket_attempts.sql", "241_codex_ticket_lifecycle.sql"} {
		migration, err := dbmigrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO codex_ticket_attempts(account_id,model,occurred_at,outcome,source) VALUES (1,'old',NOW(),'success','diagnostic'),(1,'old',NOW(),'error','diagnostic')`)
	require.NoError(t, err)
	migration, err := dbmigrations.FS.ReadFile("252_codex_attempt_ws_probe.sql")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	var oldUnknown int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM codex_ticket_attempts WHERE safety_buffering_enabled IS NULL AND safety_buffering_faster_model IS NULL AND turn_state_len IS NULL AND transport IS NULL`).Scan(&oldUnknown))
	require.Equal(t, 2, oldUnknown)
	rows, err := tx.QueryContext(ctx, `SELECT c.relname, count(a.attname) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace LEFT JOIN pg_attribute a ON a.attrelid=c.oid AND a.attname IN ('safety_buffering_enabled','safety_buffering_faster_model','turn_state_len','transport') WHERE n.nspname=current_schema() AND c.relkind IN ('r','p') AND c.relname LIKE 'codex_ticket_attempts%' GROUP BY c.relname`)
	require.NoError(t, err)
	defer rows.Close()
	partitions := 0
	for rows.Next() {
		var name string
		var columns int
		require.NoError(t, rows.Scan(&name, &columns))
		require.Equal(t, 4, columns, name)
		partitions++
	}
	require.NoError(t, rows.Err())
	require.Greater(t, partitions, 3, "parent, success, failed and daily partitions inherit the fields")
}

func TestWSProbeRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewCodexTicketAttemptRepository(integrationDB)
	accountID := time.Now().UnixNano()
	t.Cleanup(func() { _, err := repo.Delete(ctx, accountID, "", ""); require.NoError(t, err) })
	enabled, disabled := true, false
	length, zero := 780, 0
	fixtures := []service.CodexTicketAttempt{
		{Model: "normal", Outcome: "success", SafetyBufferingEnabled: &enabled, SafetyBufferingFasterModel: "gpt-6-luna", TurnStateLen: &length, Transport: "ws"},
		{Model: "false", Outcome: "error", SafetyBufferingEnabled: &disabled, TurnStateLen: &zero, Transport: "ws"},
		{Model: "http", Outcome: "miss", Transport: "http"},
		{Model: "modeltrace", Outcome: "success"},
	}
	for i := range fixtures {
		a := &fixtures[i]
		a.AccountID = accountID
		a.OccurredAt = time.Now().UTC()
		a.Trigger = "diagnostic"
		require.NoError(t, repo.Insert(ctx, a))
	}
	rows, total, err := repo.List(ctx, accountID, "", "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(4), total)
	for _, got := range rows {
		for _, want := range fixtures {
			if got.ID != want.ID {
				continue
			}
			require.Equal(t, want.SafetyBufferingEnabled, got.SafetyBufferingEnabled)
			require.Equal(t, want.SafetyBufferingFasterModel, got.SafetyBufferingFasterModel)
			require.Equal(t, want.TurnStateLen, got.TurnStateLen)
			require.Equal(t, want.Transport, got.Transport)
		}
	}
}
