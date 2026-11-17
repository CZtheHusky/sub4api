//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type creditOverageRecordingRepo struct {
	mockAccountRepoForGemini
	updates chan map[string]any
}

func (m *creditOverageRecordingRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if m.updates != nil {
		select {
		case m.updates <- updates:
		default:
		}
	}
	return nil
}

func newCreditOverageTestService() (*OpenAIGatewayService, *creditOverageRecordingRepo) {
	repo := &creditOverageRecordingRepo{updates: make(chan map[string]any, 8)}
	svc := &OpenAIGatewayService{
		accountRepo:                  repo,
		codexCreditsSnapshotThrottle: newAccountWriteThrottle(time.Hour),
	}
	return svc, repo
}

func waitForCreditOverageUpdate(t *testing.T, repo *creditOverageRecordingRepo) map[string]any {
	t.Helper()
	select {
	case updates := <-repo.updates:
		return updates
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for UpdateExtra call")
		return nil
	}
}

func creditOverageSnapshotExtra(hasCredits, unlimited bool, balance string) map[string]any {
	credits := map[string]any{"has_credits": hasCredits, "unlimited": unlimited}
	if balance != "" {
		credits["balance"] = balance
	}
	return map[string]any{"credits": credits, "fetched_at": time.Now().Unix()}
}

func TestParseCodexCreditsHeaders(t *testing.T) {
	t.Run("完整头（上游首字母大写布尔）", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("x-codex-credits-has-credits", "True")
		headers.Set("x-codex-credits-unlimited", "False")
		headers.Set("x-codex-credits-balance", "62500.50")

		credits := parseCodexCreditsHeaders(headers)
		require.NotNil(t, credits)
		require.True(t, credits.HasCredits)
		require.False(t, credits.Unlimited)
		require.NotNil(t, credits.Balance)
		require.Equal(t, "62500.50", *credits.Balance)
	})

	t.Run("小写布尔", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("x-codex-credits-has-credits", "false")
		headers.Set("x-codex-credits-unlimited", "true")

		credits := parseCodexCreditsHeaders(headers)
		require.NotNil(t, credits)
		require.False(t, credits.HasCredits)
		require.True(t, credits.Unlimited)
		require.Nil(t, credits.Balance)
	})

	t.Run("仅余额也构成有效快照", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("x-codex-credits-balance", "0")

		credits := parseCodexCreditsHeaders(headers)
		require.NotNil(t, credits)
		require.False(t, credits.HasCredits)
		require.NotNil(t, credits.Balance)
		require.Equal(t, "0", *credits.Balance)
	})

	t.Run("非法布尔视为缺失，全缺返回 nil", func(t *testing.T) {
		require.Nil(t, parseCodexCreditsHeaders(nil))

		garbage := http.Header{}
		garbage.Set("x-codex-credits-has-credits", "yes")
		garbage.Set("x-codex-credits-unlimited", "1 maybe")
		require.Nil(t, parseCodexCreditsHeaders(garbage))

		empty := http.Header{}
		empty.Set("content-type", "application/json")
		require.Nil(t, parseCodexCreditsHeaders(empty))
	})
}

// 额度打满后是否摘除调度，直接决定 credits 接管是否成立：账号被摘除就再也收不到
// 流量，credits 无从被上游计费。豁免必须压过所有摘除路径（auto-reset 阈值与
// auto-pause 阈值），而 credits 不可用时必须回到原有摘除行为。
func TestShouldAutoPauseOpenAIAccountByQuota_CreditOverage(t *testing.T) {
	now := time.Now().UTC()
	// 5h 已到 100%，同时越过用卡阈值（1.0）与暂停阈值（0.8）。
	baseExtra := func() map[string]any {
		return map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"auto_pause_5h_threshold":                0.8,
			"auto_pause_7d_disabled":                 true,
			"codex_5h_used_percent":                  100.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
		}
	}

	t.Run("开关开且有 credits：打满也不摘除", func(t *testing.T) {
		extra := baseExtra()
		extra[OpenAICreditOverageEnabledExtraKey] = true
		extra[openaiQuotaCreditsKey] = creditOverageSnapshotExtra(true, false, "62500")
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}

		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused, "credit-covered account must stay schedulable at 100% usage")
	})

	t.Run("开关开且 unlimited：同样豁免", func(t *testing.T) {
		extra := baseExtra()
		extra[OpenAICreditOverageEnabledExtraKey] = true
		extra[openaiQuotaCreditsKey] = creditOverageSnapshotExtra(false, true, "")
		account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}

		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
	})

	t.Run("开关开但 credits 耗尽：维持摘除并触发自动用卡路径", func(t *testing.T) {
		extra := baseExtra()
		extra[OpenAICreditOverageEnabledExtraKey] = true
		extra[openaiQuotaCreditsKey] = creditOverageSnapshotExtra(false, false, "0")
		account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}

		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_pending_5h", decision.reason)
	})

	t.Run("开关关（默认）：即使有 credits 也维持现状摘除", func(t *testing.T) {
		extra := baseExtra()
		extra[openaiQuotaCreditsKey] = creditOverageSnapshotExtra(true, false, "62500")
		account := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}

		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_pending_5h", decision.reason)
	})

	t.Run("开关仅对 OpenAI OAuth 母账号生效", func(t *testing.T) {
		extra := map[string]any{
			OpenAICreditOverageEnabledExtraKey: true,
			openaiQuotaCreditsKey:              creditOverageSnapshotExtra(true, false, "1"),
		}
		require.False(t, openAICreditOverageActive(&Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: extra}))
		require.False(t, openAICreditOverageActive(&Account{ID: 6, Platform: PlatformGrok, Type: AccountTypeOAuth, Extra: extra}))
		shadow := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra, ParentAccountID: ptrAccountID(100)}
		require.True(t, shadow.IsShadow())
		require.False(t, openAICreditOverageActive(shadow))
	})
}

func ptrAccountID(id int64) *int64 {
	return &id
}

// quota 型 429 是"credits 也救不了"的权威信号：不清快照会让豁免继续放行，账号在
// 冷却期内反复被调度、反复 429。瞬时 429 与未开启接管的账号不得误伤余额展示。
func TestMarkOpenAIOAuth429RateLimited_CreditOverage(t *testing.T) {
	quota429Headers := func() http.Header {
		headers := http.Header{}
		headers.Set("x-codex-primary-used-percent", "100")
		headers.Set("x-codex-primary-reset-after-seconds", "604800")
		headers.Set("x-codex-primary-window-minutes", "10080")
		return headers
	}
	overageAccount := func(id int64) *Account {
		return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
			OpenAICreditOverageEnabledExtraKey: true,
			openaiQuotaCreditsKey:              creditOverageSnapshotExtra(true, false, "62500"),
		}}
	}

	t.Run("quota 429 置 has_credits/unlimited=false 并保留余额", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()
		account := overageAccount(501)

		svc.markOpenAIOAuth429RateLimited(context.Background(), account, quota429Headers(), nil)

		updates := waitForCreditOverageUpdate(t, repo)
		snapshot, ok := updates[openaiQuotaCreditsKey].(openAICreditsSnapshot)
		require.True(t, ok)
		require.False(t, snapshot.Credits.HasCredits)
		require.False(t, snapshot.Credits.Unlimited)
		require.NotNil(t, snapshot.Credits.Balance)
		require.Equal(t, "62500", *snapshot.Credits.Balance)
	})

	t.Run("瞬时 429 不清 credits", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()
		account := overageAccount(502)

		svc.markOpenAIOAuth429RateLimited(context.Background(), account, http.Header{}, []byte(`{"detail":"Rate limit exceeded"}`))

		select {
		case updates := <-repo.updates:
			t.Fatalf("transient 429 must not touch credits snapshot, got %v", updates)
		case <-time.After(300 * time.Millisecond):
		}
	})

	t.Run("未开启接管的账号 quota 429 不清 credits", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()
		account := &Account{ID: 503, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
			openaiQuotaCreditsKey: creditOverageSnapshotExtra(true, false, "62500"),
		}}

		svc.markOpenAIOAuth429RateLimited(context.Background(), account, quota429Headers(), nil)

		select {
		case updates := <-repo.updates:
			t.Fatalf("non-overage account must not touch credits snapshot, got %v", updates)
		case <-time.After(300 * time.Millisecond):
		}
	})
}

// 热路径快照是豁免判定的输入：不落库则 has_credits 变化（尤其是耗尽）对调度不可见。
func TestUpdateCodexCreditsSnapshot(t *testing.T) {
	t.Run("写入完整快照", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()
		headers := http.Header{}
		headers.Set("x-codex-credits-has-credits", "True")
		headers.Set("x-codex-credits-unlimited", "False")
		headers.Set("x-codex-credits-balance", "120.5")

		svc.updateCodexCreditsSnapshot(context.Background(), 601, headers)

		updates := waitForCreditOverageUpdate(t, repo)
		snapshot, ok := updates[openaiQuotaCreditsKey].(openAICreditsSnapshot)
		require.True(t, ok)
		require.True(t, snapshot.Credits.HasCredits)
		require.False(t, snapshot.Credits.Unlimited)
		require.Equal(t, "120.5", *snapshot.Credits.Balance)
		require.NotZero(t, snapshot.FetchedAt)
	})

	t.Run("节流窗口内只落一次", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()
		headers := http.Header{}
		headers.Set("x-codex-credits-has-credits", "True")

		svc.updateCodexCreditsSnapshot(context.Background(), 602, headers)
		waitForCreditOverageUpdate(t, repo)
		svc.updateCodexCreditsSnapshot(context.Background(), 602, headers)

		select {
		case updates := <-repo.updates:
			t.Fatalf("throttled second write must be suppressed, got %v", updates)
		case <-time.After(300 * time.Millisecond):
		}
	})

	t.Run("无 credits 头时不写", func(t *testing.T) {
		svc, repo := newCreditOverageTestService()

		svc.updateCodexCreditsSnapshot(context.Background(), 603, http.Header{})

		select {
		case updates := <-repo.updates:
			t.Fatalf("missing x-codex-credits-* headers must not write, got %v", updates)
		case <-time.After(300 * time.Millisecond):
		}
	})
}
