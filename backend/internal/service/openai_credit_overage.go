package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OpenAICreditOverageEnabledExtraKey 是账号级 credits 接管开关。开启后，5h/7d 额度
// 打满时不再把账号摘出调度：ChatGPT 后端会自动按账号点数余额（credits）继续计费，
// 网关不发起任何消费动作（区别于 rate_limit_reset_credits 重置卡）。默认关闭，避免
// 静默消耗账号余额。
const OpenAICreditOverageEnabledExtraKey = "credit_overage_enabled"

// openAICreditOverageEnabled 报告账号是否开启 credits 接管。仅 OpenAI OAuth 母账号
// 有效：credits 是 chatgpt.com 后端概念，API Key 账号与 spark 影子不适用（影子自身
// 不持有额度语义，豁免由母账号的快照决定）。
func openAICreditOverageEnabled(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.IsShadow() {
		return false
	}
	return resolveAccountExtraBool(account.Extra, OpenAICreditOverageEnabledExtraKey)
}

// openAICreditOverageActive 报告 credits 接管当前是否实际生效：开关开启，且最近一次
// credits 快照显示账号可用余额（has_credits 或 unlimited）。快照由热路径响应头与
// 额度查询写入；quota 型 429 会把 has_credits/unlimited 置 false（被拒信号优先于
// 余额展示），使接管自动失效并回到原有的摘除+自动用卡路径。
func openAICreditOverageActive(account *Account) bool {
	if !openAICreditOverageEnabled(account) {
		return false
	}
	credits := openAICreditsFromExtra(account.Extra)
	return credits != nil && (credits.HasCredits || credits.Unlimited)
}

// openAICreditsFromExtra 从账号 Extra 解码 codex_credits_snapshot；缺失或损坏返回 nil。
func openAICreditsFromExtra(extra map[string]any) *OpenAICredits {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[openaiQuotaCreditsKey]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snapshot openAICreditsSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil || snapshot.Credits == nil {
		return nil
	}
	return snapshot.Credits
}

// parseCodexCreditsHeaders 解析推理响应携带的 x-codex-credits-* 头。布尔头上游以
// 首字母大写的 "True"/"False" 字符串下发，需容错解析；balance 保留十进制字符串以
// 避免精度损失。三个头全缺时返回 nil（如非 chatgpt.com 上游）。
func parseCodexCreditsHeaders(headers http.Header) *OpenAICredits {
	if headers == nil {
		return nil
	}
	parseBool := func(key string) (bool, bool) {
		value := strings.TrimSpace(headers.Get(key))
		if value == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, false
		}
		return parsed, true
	}
	hasCredits, hasHasCredits := parseBool("x-codex-credits-has-credits")
	unlimited, hasUnlimited := parseBool("x-codex-credits-unlimited")
	balance := strings.TrimSpace(headers.Get("x-codex-credits-balance"))
	if !hasHasCredits && !hasUnlimited && balance == "" {
		return nil
	}
	credits := &OpenAICredits{HasCredits: hasCredits, Unlimited: unlimited}
	if balance != "" {
		balanceCopy := balance
		credits.Balance = &balanceCopy
	}
	return credits
}

// markOpenAICreditsExhausted 在 credits 接管账号收到 quota 型 429 时落库
// has_credits/unlimited=false（保留 balance 供展示），让 openAICreditOverageActive
// 立即失效。异步写入，不阻塞 429 处理热路径；后续成功响应头或额度查询会恢复真实状态。
func (s *OpenAIGatewayService) markOpenAICreditsExhausted(account *Account) {
	if s == nil || s.accountRepo == nil || account == nil || account.ID <= 0 {
		return
	}
	credits := openAICreditsFromExtra(account.Extra)
	if credits == nil {
		credits = &OpenAICredits{}
	}
	credits.HasCredits = false
	credits.Unlimited = false
	snapshot := openAICreditsSnapshot{Credits: credits, FetchedAt: time.Now().Unix()}
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, account.ID, map[string]any{
			openaiQuotaCreditsKey: snapshot,
		})
	}()
}
