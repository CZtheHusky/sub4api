//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/MACOS-DO/sub4api/migrations"
	"github.com/stretchr/testify/require"
)

// TestMigration251DropsCodexTicketData 在真实 PG 上执行 251：打票设置键删除、
// extra 打票键（含 codex_turn_ticket: 前缀）清理、invalidations 表删除，
// prompt_template 与 codex_ticket_attempts 保留。
func TestMigration251DropsCodexTicketData(t *testing.T) {
	data, err := dbmigrations.FS.ReadFile("251_drop_codex_ticket.sql")
	require.NoError(t, err)
	tx := testTx(t)
	ctx := context.Background()
	_, err = tx.ExecContext(ctx, `
		CREATE TEMP TABLE settings (
			key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		) ON COMMIT DROP;
		CREATE TEMP TABLE accounts (id BIGINT PRIMARY KEY, extra JSONB) ON COMMIT DROP;
		CREATE TEMP TABLE codex_ticket_invalidations (id BIGINT PRIMARY KEY) ON COMMIT DROP;
		CREATE TEMP TABLE codex_ticket_attempts (id BIGINT PRIMARY KEY) ON COMMIT DROP;
		INSERT INTO settings(key, value) VALUES
			('openai_codex_ticket_enabled', 'true'),
			('openai_codex_ticket_allow_without_ticket', 'false'),
			('openai_codex_ticket_harvest_proxy_url', 'socks5h://proxy'),
			('openai_codex_ticket_proxy_pool', '[]'),
			('openai_codex_ticket_prompt_template', 'custom template'),
			('site_name', 'gateway');
		INSERT INTO codex_ticket_invalidations(id) VALUES (1);
		INSERT INTO codex_ticket_attempts(id) VALUES (1);
		INSERT INTO accounts(id, extra) VALUES
			(1, '{"codex_ticket_harvest_enabled":true,"codex_ticket_harvest_models":{"gpt-6-astra":false},"codex_allow_without_ticket":false,"codex_ticket_ready_models":["gpt-6-astra"],"codex_harvest_proxy_url":"socks5h://p","codex_turn_ticket:gpt-6-astra":{"state":"secret"},"keep":"me"}'),
			(2, '{"codex_turn_ticket:gpt-5.6-sol":{"state":"secret"},"ordinary":1}'),
			(3, '{"untouched":true}'),
			(4, NULL);`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, string(data))
	require.NoError(t, err)

	// 打票设置键删除，prompt_template 与无关设置保留。
	var remaining int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key LIKE 'openai_codex_ticket%'`).Scan(&remaining))
	require.Equal(t, 1, remaining, "only openai_codex_ticket_prompt_template survives")
	var template, siteName string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='openai_codex_ticket_prompt_template'`).Scan(&template))
	require.Equal(t, "custom template", template)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='site_name'`).Scan(&siteName))
	require.Equal(t, "gateway", siteName)

	// invalidations 表删除，attempts 表保留。
	var invalidationsGone bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*)=0 FROM pg_class WHERE relname='codex_ticket_invalidations' AND relnamespace=pg_my_temp_schema()`).Scan(&invalidationsGone))
	require.True(t, invalidationsGone)
	var attempts int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM codex_ticket_attempts`).Scan(&attempts))
	require.Equal(t, 1, attempts)

	// extra 打票键（含前缀键）清理干净，其它键原样保留。
	var extra1, extra2, extra3 string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra::text FROM accounts WHERE id=1`).Scan(&extra1))
	require.JSONEq(t, `{"keep":"me"}`, extra1)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra::text FROM accounts WHERE id=2`).Scan(&extra2))
	require.JSONEq(t, `{"ordinary":1}`, extra2)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT extra::text FROM accounts WHERE id=3`).Scan(&extra3))
	require.JSONEq(t, `{"untouched":true}`, extra3)
}
