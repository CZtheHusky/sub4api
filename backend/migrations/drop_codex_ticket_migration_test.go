package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration251DropsCodexTicketLeftovers(t *testing.T) {
	content, err := FS.ReadFile("251_drop_codex_ticket.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "DROP TABLE IF EXISTS codex_ticket_invalidations")
	// codex_ticket_attempts 改作降智检测历史表：除头注释外不得出现任何 DDL。
	require.NotContains(t, sql, "DROP TABLE IF EXISTS codex_ticket_attempts")
	require.NotContains(t, sql, "ALTER TABLE codex_ticket_attempts")
	require.NotContains(t, sql, "DELETE FROM codex_ticket_attempts")
	require.NotContains(t, sql, "TRUNCATE codex_ticket_attempts")

	// 打票设置键全部删除，prompt_template（降智检测模板）绝不出现在删除清单。
	for _, key := range []string{
		"openai_codex_ticket_enabled",
		"openai_codex_ticket_allow_without_ticket",
		"openai_codex_ticket_harvest_proxy_url",
		"openai_codex_ticket_proxy_pool",
	} {
		require.Contains(t, sql, "'"+key+"'")
	}
	require.Contains(t, sql, "DELETE FROM settings")
	require.NotContains(t, sql, "'openai_codex_ticket_prompt_template'")

	// extra 打票键：固定键 + codex_turn_ticket: 前缀都必须在清理谓词里。
	for _, key := range []string{
		"codex_ticket_harvest_enabled",
		"codex_ticket_harvest_models",
		"codex_allow_without_ticket",
		"codex_ticket_ready_models",
		"codex_harvest_proxy_url",
	} {
		require.Contains(t, sql, "'"+key+"'")
	}
	require.Contains(t, sql, `codex\_turn\_ticket:%`)
	require.Contains(t, sql, "jsonb_object_agg")
	require.Contains(t, sql, "jsonb_each")
	// 只更新确实含打票键的行。
	require.Contains(t, sql, "extra ?| ARRAY[")
	require.Contains(t, sql, "jsonb_object_keys")
}

// TestMigration251ExtraCleanupPredicateCoversAllKeys 确保 NOT IN 清单与 WHERE
// 触发清单一致——漏一侧会出现"行被扫到但没清干净"或"键永远清不掉"。
func TestMigration251ExtraCleanupPredicateCoversAllKeys(t *testing.T) {
	content, err := FS.ReadFile("251_drop_codex_ticket.sql")
	require.NoError(t, err)
	sql := string(content)
	keyPattern := regexp.MustCompile(`'(codex_[a-z_]+)'`)
	found := map[string]int{}
	for _, match := range keyPattern.FindAllStringSubmatch(sql, -1) {
		found[match[1]]++
	}
	for _, key := range []string{
		"codex_ticket_harvest_enabled",
		"codex_ticket_harvest_models",
		"codex_allow_without_ticket",
		"codex_ticket_ready_models",
		"codex_harvest_proxy_url",
	} {
		require.Equal(t, 2, found[key], "%s must appear in both NOT IN and ARRAY trigger lists", key)
	}
	require.False(t, strings.Contains(sql, "codex_turn_ticket'"), "turn-ticket keys are matched by prefix, never enumerated")
}
