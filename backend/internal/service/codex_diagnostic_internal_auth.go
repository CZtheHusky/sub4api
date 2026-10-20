package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// CodexDiagnosticInternalAuth 是进程级内部诊断令牌：启动时用 crypto/rand 生成，
// 只存内存、不出进程。降智探测请求以它为 Bearer 凭据打进本进程 gin 引擎，
// apiKeyAuth 在 ctx 诊断标记 + 令牌常量时间相等双条件满足时合成认证身份，
// 外部请求无法注入 ctx 标记，令牌也不落盘，双重防线保证零计费探测无法被伪造。
type CodexDiagnosticInternalAuth struct {
	token string
}

func NewCodexDiagnosticInternalAuth() (*CodexDiagnosticInternalAuth, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate codex diagnostic internal token: %w", err)
	}
	return &CodexDiagnosticInternalAuth{token: hex.EncodeToString(buf)}, nil
}

// ProvideCodexDiagnosticInternalAuth 是 wire provider。
func ProvideCodexDiagnosticInternalAuth() (*CodexDiagnosticInternalAuth, error) {
	return NewCodexDiagnosticInternalAuth()
}

// Token 返回令牌原文，仅供本进程内构造探测请求使用。
func (a *CodexDiagnosticInternalAuth) Token() string {
	if a == nil {
		return ""
	}
	return a.token
}

// MatchesToken 常量时间比较。
func (a *CodexDiagnosticInternalAuth) MatchesToken(token string) bool {
	if a == nil || a.token == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a.token), []byte(token)) == 1
}

type codexDiagnosticGroupKey struct{}

// WithCodexDiagnosticGroup 把诊断目标分组挂到 ctx（跨进程内 httptest 回放传递）。
func WithCodexDiagnosticGroup(ctx context.Context, groupID int64) context.Context {
	return context.WithValue(ctx, codexDiagnosticGroupKey{}, groupID)
}

// CodexDiagnosticGroupFromContext 读取诊断目标分组。
func CodexDiagnosticGroupFromContext(ctx context.Context) (int64, bool) {
	groupID, ok := ctx.Value(codexDiagnosticGroupKey{}).(int64)
	return groupID, ok && groupID > 0
}

// IsCodexTicketDiagnostic 报告 ctx 是否携带降智探测钉账号标记（导出供
// middleware/handler 子包做策略豁免判定）。
func IsCodexTicketDiagnostic(ctx context.Context) bool {
	return isCodexTicketDiagnostic(ctx)
}

// codexTicketDiagnosticTargetAccount 读取钉账号目标（service 包内调度豁免用）。
func codexTicketDiagnosticTargetAccount(ctx context.Context) (int64, bool) {
	accountID, ok := ctx.Value(codexTicketDiagnosticTargetKey{}).(int64)
	return accountID, ok && accountID > 0
}

// SyntheticAPIKey 构造诊断探测用的合成身份：Key ID=0、用户 ID=0、分组为诊断
// 目标分组（平台 openai）。合成 Key 不落库，计费执行层对诊断请求整块跳过。
func (a *CodexDiagnosticInternalAuth) SyntheticAPIKey(groupID int64) *APIKey {
	return &APIKey{
		ID:     0,
		UserID: 0,
		Name:   "codex-diagnostic-internal",
		Status: StatusAPIKeyActive,
		GroupID: func() *int64 {
			id := groupID
			return &id
		}(),
		Group: &Group{
			ID:             groupID,
			Name:           "codex-diagnostic-internal",
			Platform:       PlatformOpenAI,
			RateMultiplier: 1,
			Status:         StatusActive,
			Hydrated:       true,
		},
		User: &User{
			ID:     0,
			Status: StatusActive,
		},
	}
}
