//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type diagnosticTestAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *diagnosticTestAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if account, ok := r.accounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func TestResolveCodexDiagnosticGroup(t *testing.T) {
	t.Run("no groups", func(t *testing.T) {
		_, ok := ResolveCodexDiagnosticGroup(&Account{ID: 1})
		require.False(t, ok)
	})
	t.Run("picks smallest openai group id", func(t *testing.T) {
		account := &Account{ID: 1, AccountGroups: []AccountGroup{
			{GroupID: 9, Group: &Group{ID: 9, Platform: PlatformAnthropic}},
			{GroupID: 7, Group: &Group{ID: 7, Platform: PlatformOpenAI}},
			{GroupID: 3, Group: &Group{ID: 3, Platform: PlatformOpenAI}},
		}}
		groupID, ok := ResolveCodexDiagnosticGroup(account)
		require.True(t, ok)
		require.Equal(t, int64(3), groupID)
	})
	t.Run("only non-openai groups", func(t *testing.T) {
		account := &Account{ID: 1, AccountGroups: []AccountGroup{
			{GroupID: 9, Group: &Group{ID: 9, Platform: PlatformAnthropic}},
		}}
		_, ok := ResolveCodexDiagnosticGroup(account)
		require.False(t, ok)
	})
	t.Run("fallback to Groups edge", func(t *testing.T) {
		account := &Account{ID: 1, Groups: []*Group{{ID: 5, Platform: PlatformOpenAI}}}
		groupID, ok := ResolveCodexDiagnosticGroup(account)
		require.True(t, ok)
		require.Equal(t, int64(5), groupID)
	})
}

func TestCodexDiagnosticOutput(t *testing.T) {
	t.Run("sse deltas with completed", func(t *testing.T) {
		raw := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"1, 2\"}\n\n" +
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\", 3\"}\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{}}\n\n" +
			"data: [DONE]\n\n"
		output, complete := codexDiagnosticOutput([]byte(raw))
		require.True(t, complete)
		require.Equal(t, "1, 2, 3", output)
	})
	t.Run("sse without completed is incomplete", func(t *testing.T) {
		raw := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"1\"}\n\n"
		output, complete := codexDiagnosticOutput([]byte(raw))
		require.False(t, complete)
		require.Equal(t, "1", output)
	})
	t.Run("sse completed fallback reads response output", func(t *testing.T) {
		raw := "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"42\"}]}]}}\n\n"
		output, complete := codexDiagnosticOutput([]byte(raw))
		require.True(t, complete)
		require.Equal(t, "42", output)
	})
	t.Run("non-stream json completed", func(t *testing.T) {
		output, complete := codexDiagnosticOutput([]byte(`{"status":"completed","output":[{"content":[{"text":"7"}]}]}`))
		require.True(t, complete)
		require.Equal(t, "7", output)
	})
	t.Run("non-stream json not completed", func(t *testing.T) {
		_, complete := codexDiagnosticOutput([]byte(`{"status":"in_progress"}`))
		require.False(t, complete)
	})
	t.Run("garbage", func(t *testing.T) {
		_, complete := codexDiagnosticOutput([]byte("not json at all"))
		require.False(t, complete)
	})
}

func TestCodexDiagnosticGatewayError(t *testing.T) {
	require.Equal(t, "no_available_accounts", codexDiagnosticGatewayError([]byte(`{"reason":"no_available_accounts"}`)))
	require.Equal(t, "model_not_found", codexDiagnosticGatewayError([]byte(`{"error":{"code":"model_not_found"}}`)))
	require.Empty(t, codexDiagnosticGatewayError([]byte(`{"reason":"bad code with spaces"}`)))
	require.Empty(t, codexDiagnosticGatewayError([]byte(`{"reason":"`+string(make([]byte, 65))+`"}`)))
	require.Empty(t, codexDiagnosticGatewayError([]byte("not json")))
}

func newDiagnosticTestRunner(gateway *OpenAIGatewayService, repo AccountRepository, engine http.Handler) *CodexGatewayDiagnosticRunner {
	auth, _ := NewCodexDiagnosticInternalAuth()
	runner := &CodexGatewayDiagnosticRunner{
		auth:        auth,
		gateway:     gateway,
		accountRepo: repo,
		stopCh:      make(chan struct{}),
	}
	runner.SetEngine(engine)
	return runner
}

func TestRunGatewayDiagnosticTemplateInvalid(t *testing.T) {
	settings := &SettingService{settingRepo: &codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyOpenAICodexTicketPromptTemplate: "invalid",
	}}}
	gateway := &OpenAIGatewayService{settingService: settings}
	engineHit := false
	engine := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { engineHit = true })
	runner := newDiagnosticTestRunner(gateway, &diagnosticTestAccountRepo{accounts: map[int64]*Account{}}, engine)

	_, _, _, reason := runner.RunGatewayDiagnostic(context.Background(), 1, "gpt-6.1-sol", ModelTraceChallenge{Prompt: "p", ExpectedCount: 3}, time.Second)
	require.Equal(t, "template_invalid", reason)
	require.False(t, engineHit, "预检失败不得打进引擎")
}

func TestRunGatewayDiagnosticNoGroup(t *testing.T) {
	settings := &SettingService{}
	gateway := &OpenAIGatewayService{settingService: settings}
	repo := &diagnosticTestAccountRepo{accounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}}
	engineHit := false
	engine := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { engineHit = true })
	runner := newDiagnosticTestRunner(gateway, repo, engine)

	_, _, _, reason := runner.RunGatewayDiagnostic(context.Background(), 1, "gpt-6.1-sol", ModelTraceChallenge{Prompt: "p", ExpectedCount: 3}, time.Second)
	require.Equal(t, "no_group", reason)
	require.False(t, engineHit)
}

func TestRunGatewayDiagnosticAccountMissing(t *testing.T) {
	settings := &SettingService{}
	gateway := &OpenAIGatewayService{settingService: settings}
	repo := &diagnosticTestAccountRepo{accounts: map[int64]*Account{}}
	runner := newDiagnosticTestRunner(gateway, repo, http.NotFoundHandler())

	_, _, _, reason := runner.RunGatewayDiagnostic(context.Background(), 99, "gpt-6.1-sol", ModelTraceChallenge{Prompt: "p", ExpectedCount: 3}, time.Second)
	require.Equal(t, "identity_resolution_failed", reason)
}

func TestCodexDiagnosticExemptsAccountBlock(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	past := time.Now().Add(-time.Minute)
	diagnosticCtx := WithCodexTicketDiagnostic(context.Background(), 41)

	blockedByAutoCheck := &Account{
		ID:                    41,
		TempUnschedulableUntil: &future,
		TempUnschedulableReason: `{"source":"auto_degradation_check","reason":"degraded"}`,
	}
	require.True(t, codexDiagnosticExemptsAccountBlock(diagnosticCtx, blockedByAutoCheck))
	require.False(t, codexDiagnosticExemptsAccountBlock(context.Background(), blockedByAutoCheck), "非诊断请求不豁免")

	otherReason := &Account{ID: 41, TempUnschedulableUntil: &future, TempUnschedulableReason: "rate limit exceeded"}
	require.False(t, codexDiagnosticExemptsAccountBlock(diagnosticCtx, otherReason), "其它来源封锁不豁免")

	alsoOverloaded := &Account{
		ID:                      41,
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: `{"source":"auto_degradation_check"}`,
		OverloadUntil:           &future,
	}
	require.False(t, codexDiagnosticExemptsAccountBlock(diagnosticCtx, alsoOverloaded), "叠加其它冷却不豁免")

	expired := &Account{ID: 41, TempUnschedulableUntil: &past, TempUnschedulableReason: `{"source":"auto_degradation_check"}`}
	require.False(t, codexDiagnosticExemptsAccountBlock(diagnosticCtx, expired))
}

func TestWithCodexDiagnosticPinnedAccount(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	blocked := &Account{
		ID:                      41,
		Platform:                PlatformOpenAI,
		Type:                    AccountTypeOAuth,
		Status:                  StatusActive,
		Schedulable:             true,
		GroupIDs:                []int64{9},
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: `{"source":"auto_degradation_check"}`,
	}
	repo := &diagnosticTestAccountRepo{accounts: map[int64]*Account{41: blocked}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	groupID := int64(9)
	diagnosticCtx := WithCodexTicketDiagnostic(context.Background(), 41)

	t.Run("appends blocked pinned account for diagnostic", func(t *testing.T) {
		accounts := svc.withCodexDiagnosticPinnedAccount(diagnosticCtx, nil, &groupID, PlatformOpenAI)
		require.Len(t, accounts, 1)
		require.Equal(t, int64(41), accounts[0].ID)
		require.Nil(t, accounts[0].TempUnschedulableUntil, "补回副本清掉本功能写入的停调字段")
		require.Empty(t, accounts[0].TempUnschedulableReason)
		// 原对象不受影响（持久化封锁语义保留）
		require.NotNil(t, blocked.TempUnschedulableUntil)
	})
	t.Run("does not append for normal requests", func(t *testing.T) {
		accounts := svc.withCodexDiagnosticPinnedAccount(context.Background(), nil, &groupID, PlatformOpenAI)
		require.Empty(t, accounts)
	})
	t.Run("does not append other-source block", func(t *testing.T) {
		otherBlocked := &Account{
			ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			GroupIDs: []int64{9}, TempUnschedulableUntil: &future, TempUnschedulableReason: "503 cooldown",
		}
		svc2 := &OpenAIGatewayService{accountRepo: &diagnosticTestAccountRepo{accounts: map[int64]*Account{42: otherBlocked}}}
		ctx := WithCodexTicketDiagnostic(context.Background(), 42)
		require.Empty(t, svc2.withCodexDiagnosticPinnedAccount(ctx, nil, &groupID, PlatformOpenAI))
	})
	t.Run("does not duplicate present account", func(t *testing.T) {
		existing := []Account{*blocked}
		accounts := svc.withCodexDiagnosticPinnedAccount(diagnosticCtx, existing, &groupID, PlatformOpenAI)
		require.Len(t, accounts, 1)
	})
	t.Run("skips group mismatch", func(t *testing.T) {
		otherGroup := int64(3)
		accounts := svc.withCodexDiagnosticPinnedAccount(diagnosticCtx, nil, &otherGroup, PlatformOpenAI)
		require.Empty(t, accounts)
	})
	t.Run("skips platform mismatch", func(t *testing.T) {
		accounts := svc.withCodexDiagnosticPinnedAccount(diagnosticCtx, nil, &groupID, PlatformAnthropic)
		require.Empty(t, accounts)
	})
}

func TestWithOpenAIProfitControlGateSkippedForDiagnostic(t *testing.T) {
	group := &Group{
		ID: 9, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
		RateMultiplier: 2, ProfitControlEnabled: true,
	}
	groupID := int64(9)
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	svc := &OpenAIGatewayService{}

	gated := svc.withOpenAIProfitControlGate(ctx, &groupID)
	_, installed := gated.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	require.True(t, installed, "非诊断请求照常装门")

	diagnosticCtx := WithCodexTicketDiagnostic(ctx, 41)
	skipped := svc.withOpenAIProfitControlGate(diagnosticCtx, &groupID)
	_, installed = skipped.Value(openAIProfitControlGateCtxKey{}).(*openAIProfitControlGate)
	require.False(t, installed, "诊断请求跳过利润门")
}

func TestIsOpenAIAccountRequestRuntimeBlockedDiagnosticExemption(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	account := &Account{
		ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: `{"source":"auto_degradation_check"}`,
	}
	svc := &OpenAIGatewayService{}
	// 制造进程内封锁，让 persisted 冷却检查生效。
	svc.openaiAccountRuntimeBlockUntil.Store(account.ID, future)
	svc.openaiAccountRuntimeBlockGeneration.Store(account.ID, uint64(1))

	diagnosticCtx := WithCodexTicketDiagnostic(context.Background(), 41)
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlockedContext(diagnosticCtx, account, "gpt-6.1-sol", false),
		"诊断请求豁免本功能封锁")
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlockedContext(context.Background(), account, "gpt-6.1-sol", false),
		"普通请求仍被封锁")
	_, stillBlocked := svc.openaiAccountRuntimeBlockUntil.Load(account.ID)
	require.True(t, stillBlocked, "豁免放行不得清除进程内封锁")
}
