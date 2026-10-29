package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MACOS-DO/sub4api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIAutoResetCreditExtra(t *testing.T) {
	t.Run("未配置的母账号默认开启", func(t *testing.T) {
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		config := ResolveOpenAIAutoResetCreditConfig(account)
		require.True(t, config.Enabled)
		require.Equal(t, 1.0, config.Threshold5h)
		require.Equal(t, 1.0, config.Threshold7d)
	})

	t.Run("显式关闭和不支持的账号不启用", func(t *testing.T) {
		parentID := int64(1)
		for _, account := range []*Account{
			nil,
			{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			{Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID},
			{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAIAutoResetCreditEnabledExtraKey: false}},
		} {
			require.False(t, ResolveOpenAIAutoResetCreditConfig(account).Enabled)
		}
	})

	t.Run("仅设置阈值时沿用默认开启并补齐另一窗口", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCredit5hThresholdExtraKey: 0.95,
		})
		require.NoError(t, err)
		config := ResolveOpenAIAutoResetCreditConfig(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra})
		require.True(t, config.Enabled)
		require.Equal(t, 0.95, config.Threshold5h)
		require.Equal(t, 1.0, config.Threshold7d)
	})

	t.Run("开启时补齐两个百分百阈值并剥离运行态", func(t *testing.T) {
		extra, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey: true,
			OpenAIAutoResetCreditStateExtraKey:   map[string]any{"status": "success"},
		})
		require.NoError(t, err)
		require.Equal(t, 1.0, extra[OpenAIAutoResetCredit5hThresholdExtraKey])
		require.Equal(t, 1.0, extra[OpenAIAutoResetCredit7dThresholdExtraKey])
		require.NotContains(t, extra, OpenAIAutoResetCreditStateExtraKey)
	})

	t.Run("阈值和账号类型严格校验", func(t *testing.T) {
		_, err := normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 0.0009,
		})
		require.Error(t, err)

		_, err = normalizeOpenAIAutoResetCreditExtra(PlatformOpenAI, AccountTypeOAuth, true, map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey: true,
		})
		require.Error(t, err)
	})
}

func TestShouldAutoPauseOpenAIAccountByQuota_AutoResetCreditStates(t *testing.T) {
	now := time.Now().UTC()
	baseExtra := map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:     true,
		OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
		OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
		"auto_pause_5h_threshold":                0.8,
		"auto_pause_7d_disabled":                 true,
		"codex_5h_used_percent":                  90.0,
		"codex_usage_updated_at":                 now.Format(time.RFC3339),
		"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
	}

	t.Run("卡状态未知时暂停并触发异步查询", func(t *testing.T) {
		account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: cloneOpenAIAutoResetExtra(baseExtra)}
		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_credit_check_5h", decision.reason)
	})

	t.Run("明确有卡时允许继续到用卡阈值", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339),
		}
		account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
	})

	t.Run("达到用卡阈值后即使有卡也退出调度", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra["codex_5h_used_percent"] = 100.0
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, CheckedAt: now.Format(time.RFC3339),
		}
		account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.True(t, paused)
		require.Equal(t, "quota_auto_reset_pending_5h", decision.reason)
	})

	t.Run("自然窗口重置后清除动态阻塞", func(t *testing.T) {
		extra := cloneOpenAIAutoResetExtra(baseExtra)
		extra["codex_5h_used_percent"] = 100.0
		extra["codex_5h_reset_at"] = now.Add(-time.Second).Format(time.RFC3339)
		extra[OpenAIAutoResetCreditStateExtraKey] = OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusFailed, TriggerWindow: "5h", ErrorCode: "RESET_FAILED",
		}
		account := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
		paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
		require.False(t, paused)
	})
}

func TestNotifyOpenAIAutoResetFromScheduler_CoolsDownPerAccount(t *testing.T) {
	svc := &OpenAIQuotaAutoResetService{ctx: context.Background(), queue: make(chan int64, 8)}
	setOpenAIAutoResetNotifier(svc)
	t.Cleanup(func() { clearOpenAIAutoResetNotifier(svc) })

	const accountA, accountB int64 = 9_900_001, 9_900_002
	t.Cleanup(func() {
		openAIAutoResetSchedulerNotifiedAt.Delete(accountA)
		openAIAutoResetSchedulerNotifiedAt.Delete(accountB)
	})
	drain := func() []int64 {
		var got []int64
		for {
			select {
			case id := <-svc.queue:
				svc.pending.Delete(id)
				got = append(got, id)
			default:
				return got
			}
		}
	}

	base := time.Now()
	require.True(t, notifyOpenAIAutoResetFromSchedulerAt(accountA, base))
	require.Equal(t, []int64{accountA}, drain())

	require.False(t, notifyOpenAIAutoResetFromSchedulerAt(accountA, base.Add(10*time.Second)), "冷却期内同一账号不重复通知")
	require.Empty(t, drain())

	require.True(t, notifyOpenAIAutoResetFromSchedulerAt(accountB, base.Add(10*time.Second)), "冷却按账号独立")
	require.Equal(t, []int64{accountB}, drain())

	require.True(t, notifyOpenAIAutoResetFromSchedulerAt(accountA, base.Add(openAIAutoResetSchedulerNotifyCooldown)))
	require.Equal(t, []int64{accountA}, drain())
}

func TestSelectOpenAIAutoResetCandidate_FailsClosed(t *testing.T) {
	candidates := []openAIAutoResetCreditCandidate{
		{ID: "later", ExpiresAt: "2026-09-02T00:00:00Z"},
		{ID: "earlier", ExpiresAt: "2026-09-01T00:00:00Z"},
	}
	selected, err := selectOpenAIAutoResetCandidate(candidates, 2, nil, "cycle-a")
	require.NoError(t, err)
	require.Equal(t, "earlier", selected.ID)

	_, err = selectOpenAIAutoResetCandidate([]openAIAutoResetCreditCandidate{
		{ExpiresAt: "2026-09-01T00:00:00Z"},
	}, 1, nil, "cycle-a")
	require.Error(t, err)

	_, err = selectOpenAIAutoResetCandidate(candidates, 2, &OpenAIAutoResetCreditState{
		AttemptCycleHash: "cycle-a", AttemptCreditHash: shortOpenAIAutoResetHash("missing"),
	}, "cycle-a")
	require.Error(t, err, "模糊结果后原卡消失时不得切换下一张卡")
}

func TestOpenAIQuotaAutoResetService_AssessesIndependentWindows(t *testing.T) {
	service := &OpenAIQuotaAutoResetService{}
	account := &Account{Extra: map[string]any{
		"auto_pause_5h_disabled": true,
		"auto_pause_7d_disabled": true,
	}}
	config := OpenAIAutoResetCreditConfig{Enabled: true, Threshold5h: 0.8, Threshold7d: 0.9}
	tests := []struct {
		name       string
		fiveHour   float64
		sevenDay   float64
		wantWindow string
	}{
		{name: "5h", fiveHour: 0.8, sevenDay: 0.2, wantWindow: "5h"},
		{name: "7d", fiveHour: 0.2, sevenDay: 0.9, wantWindow: "7d"},
		{name: "同时触发", fiveHour: 0.95, sevenDay: 0.95, wantWindow: "5h+7d"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment := service.buildAssessment(account, config, test.fiveHour, test.sevenDay)
			require.True(t, assessment.resetReached)
			require.Equal(t, test.wantWindow, assessment.triggerWindow)
		})
	}
}

type autoResetTestAccountRepo struct {
	AccountRepository
	mu        sync.Mutex
	account   *Account
	scanTimes []time.Time
}

func (r *autoResetTestAccountRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanTimes = append(r.scanTimes, time.Now())
	account := *r.account
	account.Extra = cloneOpenAIAutoResetExtra(r.account.Extra)
	return []Account{account}, nil, nil
}

func (r *autoResetTestAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *r.account
	copy.Extra = cloneOpenAIAutoResetExtra(r.account.Extra)
	return &copy, nil
}

func (r *autoResetTestAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.Extra == nil {
		r.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	return nil
}

type autoResetTestQuota struct {
	usage        *OpenAIQuotaUsage
	resetCalls   atomic.Int32
	resetEntered chan struct{}
	releaseReset chan struct{}
	enterOnce    sync.Once
	mu           sync.Mutex
	resetArgs    [][2]string
	failFirst    bool
	queryCalls   atomic.Int32
	queryErr     error
}

func (q *autoResetTestQuota) QueryUsage(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.queryCalls.Add(1)
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	copy := *q.usage
	return &copy, nil
}

func (q *autoResetTestQuota) CacheResetCreditsSnapshot(context.Context, int64, *OpenAIRateLimitResetCredits) error {
	return nil
}

func (q *autoResetTestQuota) CachePostResetSnapshot(context.Context, int64, *OpenAIQuotaUsage) error {
	return nil
}

func (q *autoResetTestQuota) ResetCreditTargeted(_ context.Context, _ int64, creditID, redeemRequestID string) (*OpenAIQuotaResetResult, error) {
	if creditID == "" || redeemRequestID == "" {
		panic("targeted reset identifiers must be present")
	}
	call := q.resetCalls.Add(1)
	q.mu.Lock()
	q.resetArgs = append(q.resetArgs, [2]string{creditID, redeemRequestID})
	q.mu.Unlock()
	if q.failFirst && call == 1 {
		return nil, context.DeadlineExceeded
	}
	if q.resetEntered != nil {
		q.enterOnce.Do(func() { close(q.resetEntered) })
	}
	if q.releaseReset != nil {
		<-q.releaseReset
	}
	return &OpenAIQuotaResetResult{Code: "ok", WindowsReset: 2}, nil
}

type autoResetTestRecoverer struct{}

func (autoResetTestRecoverer) RecoverAccountState(context.Context, int64, AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	return &SuccessfulTestRecoveryResult{ClearedRateLimit: true}, nil
}

func TestOpenAIQuotaAutoResetService_ConcurrentInstancesConsumeOnce(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 99, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  100.0,
			"codex_7d_used_percent":                  10.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
			"codex_7d_reset_at":                      now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
	repo := &autoResetTestAccountRepo{account: account}
	usage := &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 10, LimitWindowSeconds: 7 * 24 * 60 * 60, ResetAfterSeconds: 86400, ResetAt: now.Add(24 * time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
		},
		autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "credit-sensitive-id", ExpiresAt: now.Add(48 * time.Hour).Format(time.RFC3339)}},
	}
	quota := &autoResetTestQuota{usage: usage, resetEntered: make(chan struct{}), releaseReset: make(chan struct{})}
	idempotencyRepo := newInMemoryIdempotencyRepo()
	config := DefaultIdempotencyConfig()
	config.ObserveOnly = false
	config.ProcessingTimeout = time.Second
	serviceA := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, nil)
	serviceB := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{}, NewIdempotencyCoordinator(idempotencyRepo, config), nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = serviceA.evaluateAccount(context.Background(), account.ID)
	}()
	<-quota.resetEntered
	go func() {
		defer wg.Done()
		_ = serviceB.evaluateAccount(context.Background(), account.ID)
	}()
	time.Sleep(50 * time.Millisecond)
	close(quota.releaseReset)
	wg.Wait()

	require.Equal(t, int32(1), quota.resetCalls.Load())
	repo.mu.Lock()
	state := openAIAutoResetStateFromExtra(repo.account.Extra)
	repo.mu.Unlock()
	require.NotNil(t, state)
	require.Equal(t, OpenAIAutoResetStatusSuccess, state.Status)
	encodedState, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(encodedState), "credit-sensitive-id")
}

func TestOpenAIQuotaAutoResetService_TimeoutRetryReusesRequestBody(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{
		ID: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			OpenAIAutoResetCreditEnabledExtraKey:     true,
			OpenAIAutoResetCredit5hThresholdExtraKey: 1.0,
			OpenAIAutoResetCredit7dThresholdExtraKey: 1.0,
			"codex_5h_used_percent":                  100.0,
			"codex_usage_updated_at":                 now.Format(time.RFC3339),
			"codex_5h_reset_at":                      now.Add(time.Hour).Format(time.RFC3339),
		},
	}
	repo := &autoResetTestAccountRepo{account: account}
	expiresAt := now.Add(48 * time.Hour).Format(time.RFC3339)
	quota := &autoResetTestQuota{
		failFirst: true,
		usage: &OpenAIQuotaUsage{
			FetchedAt: now.Unix(),
			RateLimit: &OpenAIRateLimit{
				PrimaryWindow: &OpenAIRateLimitWindow{UsedPercent: 100, LimitWindowSeconds: 5 * 60 * 60, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
			},
			RateLimitResetCredits: &OpenAIRateLimitResetCredits{
				AvailableCount: 1,
				Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: expiresAt}},
			},
			autoResetCandidates: []openAIAutoResetCreditCandidate{{ID: "retry-credit", ExpiresAt: expiresAt}},
		},
	}
	idempotencyConfig := DefaultIdempotencyConfig()
	idempotencyConfig.ObserveOnly = false
	idempotencyConfig.FailedRetryBackoff = 0
	service := NewOpenAIQuotaAutoResetService(
		repo,
		quota,
		autoResetTestRecoverer{},
		NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig),
		nil, nil, nil,
	)

	require.Error(t, service.evaluateAccount(context.Background(), account.ID))
	require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
	quota.mu.Lock()
	args := append([][2]string(nil), quota.resetArgs...)
	quota.mu.Unlock()
	require.Len(t, args, 2)
	require.Equal(t, args[0], args[1], "超时重试必须复用相同 credit_id 与 redeem_request_id")
}

// newResetThresholdTestFixture 构造一个 7d 用量已过用卡阈值、处于停调中的账号。
func newResetThresholdTestFixture(t *testing.T, state *OpenAIAutoResetCreditState, availableCredits int) (*OpenAIQuotaAutoResetService, *autoResetTestAccountRepo, *autoResetTestQuota) {
	t.Helper()
	now := time.Now().UTC()
	extra := map[string]any{
		OpenAIAutoResetCreditEnabledExtraKey:     true,
		OpenAIAutoResetCredit5hThresholdExtraKey: 0.95,
		OpenAIAutoResetCredit7dThresholdExtraKey: 0.95,
		"codex_7d_used_percent":                  99.0,
		"codex_usage_updated_at":                 now.Format(time.RFC3339),
		"codex_7d_reset_at":                      now.Add(48 * time.Hour).Format(time.RFC3339),
	}
	if state != nil {
		extra[OpenAIAutoResetCreditStateExtraKey] = state
	}
	account := &Account{ID: 501, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Extra: extra}
	repo := &autoResetTestAccountRepo{account: account}
	quota := &autoResetTestQuota{usage: &OpenAIQuotaUsage{
		FetchedAt: now.Unix(),
		RateLimit: &OpenAIRateLimit{
			SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: 99, LimitWindowSeconds: 7 * 24 * 60 * 60, ResetAfterSeconds: 48 * 3600, ResetAt: now.Add(48 * time.Hour).Unix()},
		},
		RateLimitResetCredits: &OpenAIRateLimitResetCredits{AvailableCount: availableCredits},
	}}
	idempotencyConfig := DefaultIdempotencyConfig()
	idempotencyConfig.ObserveOnly = false
	service := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{},
		NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig), nil, nil, nil)
	return service, repo, quota
}

func (r *autoResetTestAccountRepo) setExtraForTest(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account.Extra[key] = value
}

func (r *autoResetTestAccountRepo) stateForTest() *OpenAIAutoResetCreditState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return openAIAutoResetStateFromExtra(r.account.Extra)
}

func TestOpenAIQuotaAutoResetService_ResetThresholdSkipsRequeryWhileNoCreditIsFresh(t *testing.T) {
	now := time.Now().UTC()
	service, repo, quota := newResetThresholdTestFixture(t, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusNoCredit, TriggerWindow: "7d", CheckedAt: now.Format(time.RFC3339), ErrorCode: "NO_RESET_CREDIT",
	}, 0)

	for i := 0; i < 3; i++ {
		require.NoError(t, service.evaluateAccount(context.Background(), 501))
	}
	require.Zero(t, quota.queryCalls.Load(), "10 分钟内已确认无卡，调度通知不应再触发上游查询")

	stale := now.Add(-openAIAutoResetSnapshotTTL - time.Minute).Format(time.RFC3339)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusNoCredit, TriggerWindow: "7d", CheckedAt: stale, ErrorCode: "NO_RESET_CREDIT",
	})
	repo.setExtraForTest("codex_usage_updated_at", stale)
	require.NoError(t, service.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(1), quota.queryCalls.Load(), "无卡结论过期后应重新查询，以便发现新发放的卡")
	require.Equal(t, OpenAIAutoResetStatusNoCredit, repo.stateForTest().Status)
}

func TestOpenAIQuotaAutoResetService_ResetThresholdStillQueriesWhenCreditAvailable(t *testing.T) {
	now := time.Now().UTC()
	service, _, quota := newResetThresholdTestFixture(t, &OpenAIAutoResetCreditState{
		Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1, TriggerWindow: "7d", CheckedAt: now.Format(time.RFC3339),
	}, 0)

	require.NoError(t, service.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(1), quota.queryCalls.Load(), "有卡时到达用卡阈值必须立即查询并进入用卡流程")
}

func TestOpenAIQuotaAutoResetService_QueryFailureBacksOffBeforeRetry(t *testing.T) {
	service, repo, quota := newResetThresholdTestFixture(t, nil, 0)
	quota.queryErr = errors.New("upstream EOF")

	require.Error(t, service.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(1), quota.queryCalls.Load())
	state := repo.stateForTest()
	require.Equal(t, OpenAIAutoResetStatusFailed, state.Status)
	require.Equal(t, "RESET_CREDIT_QUERY_FAILED", state.ErrorCode)

	for i := 0; i < 3; i++ {
		require.NoError(t, service.evaluateAccount(context.Background(), 501))
	}
	require.Equal(t, int32(1), quota.queryCalls.Load(), "查询失败后一分钟内不应重复打上游")

	state.LastResultAt = time.Now().UTC().Add(-openAIAutoResetQueryFailureRetryAfter - time.Second).Format(time.RFC3339)
	repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
	quota.queryErr = nil
	require.NoError(t, service.evaluateAccount(context.Background(), 501))
	require.Equal(t, int32(2), quota.queryCalls.Load(), "重试间隔过后应恢复查询")
	require.Equal(t, OpenAIAutoResetStatusNoCredit, repo.stateForTest().Status)
}

func TestOpenAIQuotaAutoResetService_ExpiringCredit(t *testing.T) {
	tests := []struct {
		name          string
		expiresIn     time.Duration
		used5h        float64
		used7d        float64
		cachedUsed5h  float64
		upstreamDelay time.Duration
		pinLater      bool
		disabled      bool
		unsetEnabled  bool
		failFirst     bool
		wantReset     bool
		wantWindow    string
	}{
		{name: "5h 有消耗即使用临期卡", expiresIn: 4 * time.Minute, used5h: 1, wantReset: true, wantWindow: "5h"},
		{name: "未配置开关也默认使用临期卡", expiresIn: 4 * time.Minute, used5h: 1, unsetEnabled: true, wantReset: true, wantWindow: "5h"},
		{name: "5h 全满但 7d 有消耗仍用卡", expiresIn: 4 * time.Minute, used7d: 1, wantReset: true, wantWindow: "7d"},
		{name: "两个窗口都有消耗", expiresIn: 4 * time.Minute, used5h: 1, used7d: 2, wantReset: true, wantWindow: "5h+7d"},
		{name: "缓存有消耗但上游两窗口均全满则保留卡", expiresIn: 4 * time.Minute, cachedUsed5h: 90},
		{name: "超过五分钟不提前消费", expiresIn: 6 * time.Minute, used5h: 1},
		{name: "已经过期不消费", expiresIn: -time.Minute, used5h: 1},
		{name: "关闭自动用卡不消费", expiresIn: 4 * time.Minute, used5h: 1, disabled: true},
		{name: "上游确认到期时间推迟后不消费", expiresIn: 4 * time.Minute, used5h: 1, upstreamDelay: time.Hour},
		{name: "未决兑换锁定长期卡时不得临期消费或换卡", expiresIn: 4 * time.Minute, used5h: 1, pinLater: true},
		{name: "超时后按原卡和幂等键重试", expiresIn: 4 * time.Minute, used5h: 1, failFirst: true, wantReset: true, wantWindow: "5h"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			expiresAt := now.Add(test.expiresIn).Format(time.RFC3339)
			cached := &OpenAIRateLimitResetCredits{
				AvailableCount: 2,
				Credits: []OpenAIRateLimitResetCreditDetail{
					{ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
					{ExpiresAt: expiresAt},
				},
			}
			// JSON 往返模拟数据库 extra，额度头刚刷新且缓存用量为零也必须检查临期卡。
			rawExtra, err := json.Marshal(map[string]any{
				OpenAIAutoResetCreditEnabledExtraKey: !test.disabled,
				OpenAIAutoResetCreditStateExtraKey: OpenAIAutoResetCreditState{
					Status: OpenAIAutoResetStatusAvailable, AvailableCount: 2, CheckedAt: now.Format(time.RFC3339),
				},
				openaiQuotaResetCreditsKey: cached,
				"codex_usage_updated_at":   now.Format(time.RFC3339),
				"codex_5h_used_percent":    test.cachedUsed5h,
				"codex_7d_used_percent":    0.0,
			})
			require.NoError(t, err)
			var extra map[string]any
			require.NoError(t, json.Unmarshal(rawExtra, &extra))
			if test.unsetEnabled {
				delete(extra, OpenAIAutoResetCreditEnabledExtraKey)
			}
			account := &Account{ID: 601, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Extra: extra}
			repo := &autoResetTestAccountRepo{account: account}
			upstreamExpiry := now.Add(test.expiresIn + test.upstreamDelay).Format(time.RFC3339)
			quota := &autoResetTestQuota{failFirst: test.failFirst, usage: &OpenAIQuotaUsage{
				FetchedAt: now.Unix(),
				RateLimit: &OpenAIRateLimit{
					PrimaryWindow:   &OpenAIRateLimitWindow{UsedPercent: test.used5h, LimitWindowSeconds: 5 * 3600, ResetAfterSeconds: 3600, ResetAt: now.Add(time.Hour).Unix()},
					SecondaryWindow: &OpenAIRateLimitWindow{UsedPercent: test.used7d, LimitWindowSeconds: 7 * 24 * 3600, ResetAfterSeconds: 86400, ResetAt: now.Add(24 * time.Hour).Unix()},
				},
				RateLimitResetCredits: &OpenAIRateLimitResetCredits{
					AvailableCount: 2,
					Credits: []OpenAIRateLimitResetCreditDetail{
						{ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
						{ExpiresAt: upstreamExpiry},
					},
				},
				autoResetCandidates: []openAIAutoResetCreditCandidate{
					{ID: "later-credit", ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339)},
					{ID: "expiring-credit", ExpiresAt: upstreamExpiry},
				},
			}}
			if test.pinLater {
				state := repo.stateForTest()
				state.AttemptCycleHash = shortOpenAIAutoResetHash(openAIAutoResetCycleSeed(quota.usage))
				state.AttemptCreditHash = shortOpenAIAutoResetHash("later-credit")
				repo.setExtraForTest(OpenAIAutoResetCreditStateExtraKey, state)
			}
			idempotencyConfig := DefaultIdempotencyConfig()
			idempotencyConfig.ObserveOnly = false
			idempotencyConfig.FailedRetryBackoff = 0
			service := NewOpenAIQuotaAutoResetService(repo, quota, autoResetTestRecoverer{},
				NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), idempotencyConfig), nil, nil, nil)

			err = service.evaluateAccount(context.Background(), account.ID)
			if test.failFirst {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, service.evaluateAccount(context.Background(), account.ID))
			if !test.wantReset {
				require.Zero(t, quota.resetCalls.Load())
				if test.expiresIn > 0 && test.expiresIn <= 5*time.Minute && !test.disabled {
					require.Positive(t, quota.queryCalls.Load(), "临期时必须查询实际用量，不能把缓存零用量视为额度全满")
				}
				return
			}
			wantCalls := int32(1)
			if test.failFirst {
				wantCalls = 2
				require.Equal(t, quota.resetArgs[0], quota.resetArgs[1], "超时重试不能换卡或幂等键")
			}
			require.Equal(t, wantCalls, quota.resetCalls.Load(), "重复扫描不能重复消费同一张卡")
			require.Equal(t, "expiring-credit", quota.resetArgs[0][0], "优先使用最早到期的卡")
			require.Equal(t, OpenAIAutoResetStatusSuccess, repo.stateForTest().Status)
			require.Equal(t, test.wantWindow, repo.stateForTest().TriggerWindow)
		})
	}
}

func TestOpenAIQuotaAutoResetService_NonExpiringCreditDoesNotRefreshStaleSnapshots(t *testing.T) {
	service, repo, quota := newResetThresholdTestFixture(t, &OpenAIAutoResetCreditState{
		Status:    OpenAIAutoResetStatusNoCredit,
		CheckedAt: time.Now().Add(-openAIAutoResetSnapshotTTL - time.Minute).UTC().Format(time.RFC3339),
	}, 0)
	repo.setExtraForTest("codex_7d_used_percent", 0.0)
	repo.setExtraForTest("codex_usage_updated_at", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	repo.setExtraForTest(openaiQuotaResetCreditsKey, &OpenAIRateLimitResetCredits{
		AvailableCount: 1,
		Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: time.Now().Add(6 * time.Minute).UTC().Format(time.RFC3339)}},
	})
	quota.usage.RateLimit.SecondaryWindow.UsedPercent = 0

	require.NoError(t, service.evaluateAccount(context.Background(), 501))
	require.Zero(t, quota.queryCalls.Load(), "临期前只检查已知到期时间，缓存陈旧不能触发上游轮询")
}

func TestOpenAIQuotaAutoResetService_ScannerJitterChecksExpiryTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		service, repo, quota := newResetThresholdTestFixture(t, &OpenAIAutoResetCreditState{
			Status: OpenAIAutoResetStatusAvailable, AvailableCount: 1,
			CheckedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339),
		}, 1)
		credits := &OpenAIRateLimitResetCredits{
			AvailableCount: 1,
			Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(8 * time.Minute).UTC().Format(time.RFC3339)}},
		}
		repo.setExtraForTest(openaiQuotaResetCreditsKey, credits)
		repo.setExtraForTest("codex_7d_used_percent", 0.0)
		repo.setExtraForTest("codex_usage_updated_at", now.Add(-time.Hour).UTC().Format(time.RFC3339))
		quota.usage.RateLimit.SecondaryWindow.UsedPercent = 0
		quota.usage.RateLimitResetCredits = credits
		service.wg.Add(2)
		go service.runWorker()
		go service.runScanner()
		defer service.Stop()

		time.Sleep(3 * time.Minute)
		synctest.Wait()
		require.Zero(t, quota.queryCalls.Load(), "进入最后五分钟前不请求上游")
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		checks := quota.queryCalls.Load()
		require.GreaterOrEqual(t, checks, int32(2), "五分钟内至少复查两次，额度全满时继续保留卡")
		require.Zero(t, quota.resetCalls.Load())
		time.Sleep(3 * time.Minute)
		synctest.Wait()
		require.Equal(t, checks, quota.queryCalls.Load(), "卡片过期后停止临期查询")

		repo.mu.Lock()
		scans := append([]time.Time(nil), repo.scanTimes...)
		repo.mu.Unlock()
		require.GreaterOrEqual(t, len(scans), 5)
		require.Equal(t, 2*time.Second, scans[0].Sub(now), "保留启动时的首次检查")
		intervals := make(map[time.Duration]struct{})
		for i := 1; i < len(scans); i++ {
			interval := scans[i].Sub(scans[i-1])
			require.GreaterOrEqual(t, interval, 105*time.Second)
			require.LessOrEqual(t, interval, 135*time.Second)
			intervals[interval] = struct{}{}
		}
		require.Greater(t, len(intervals), 1, "每轮应重新随机间隔")
	})
}

func TestOpenAIAutoResetCreditsExpiring_FiveMinuteBoundary(t *testing.T) {
	now := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		expiresIn time.Duration
		want      bool
	}{
		{name: "恰好五分钟", expiresIn: 5 * time.Minute, want: true},
		{name: "五分钟以内", expiresIn: time.Second, want: true},
		{name: "尚未进入五分钟", expiresIn: 5*time.Minute + time.Second},
		{name: "恰好到期"},
		{name: "已过期", expiresIn: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			credits := &OpenAIRateLimitResetCredits{
				AvailableCount: 1,
				Credits:        []OpenAIRateLimitResetCreditDetail{{ExpiresAt: now.Add(test.expiresIn).Format(time.RFC3339)}},
			}
			require.Equal(t, test.want, openAIAutoResetCreditsExpiring(credits, now))
		})
	}
}
