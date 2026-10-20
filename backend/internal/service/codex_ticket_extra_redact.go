package service

import (
	"maps"
	"strings"
)

// IsOpenAICodexTicketPrivateExtraKey 判断 accounts.extra 中的打票遗留私有键
// （活门票凭据 codex_turn_ticket:* 与历史账号级代理覆盖）。打票功能已删除，
// 但存量账号数据在迁移清理（251）之前仍可能携带这些键——门票是活凭据，
// 序列化/导出前必须剥除。
func IsOpenAICodexTicketPrivateExtraKey(key string) bool {
	return strings.HasPrefix(key, "codex_turn_ticket:") || key == "codex_harvest_proxy_url"
}

// RedactOpenAICodexTicketExtra 剥除导出载荷中的打票遗留材料，不改动源对象。
func RedactOpenAICodexTicketExtra(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	redacted := maps.Clone(extra)
	for key := range redacted {
		if IsOpenAICodexTicketPrivateExtraKey(key) {
			delete(redacted, key)
		}
	}
	return redacted
}
