//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newDiagnosticAuthTestRouter(t *testing.T, auth *service.CodexDiagnosticInternalAuth, calls *atomic.Int32) (*gin.Engine, *atomic.Value) {
	t.Helper()
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) {
		calls.Add(1)
		return nil, service.ErrAPIKeyNotFound
	}}
	cfg := &config.Config{}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	var seen atomic.Value
	r := gin.New()
	r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg, auth)))
	r.POST("/v1/responses", func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		subject, ok := GetAuthSubjectFromContext(c)
		require.True(t, ok)
		seen.Store([2]any{apiKey, subject})
		c.Status(http.StatusOK)
	})
	return r, &seen
}

func diagnosticRequest(ctx context.Context, token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestCodexDiagnosticSyntheticAuthSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, err := service.NewCodexDiagnosticInternalAuth()
	require.NoError(t, err)
	var calls atomic.Int32
	r, seen := newDiagnosticAuthTestRouter(t, auth, &calls)

	ctx := service.WithCodexDiagnosticGroup(service.WithCodexTicketDiagnostic(context.Background(), 41), 9)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, diagnosticRequest(ctx, auth.Token()))

	require.Equal(t, http.StatusOK, w.Code)
	require.Zero(t, calls.Load(), "诊断合成认证不得触及 Key 缓存/DB 查询")
	captured, ok := seen.Load().([2]any)
	require.True(t, ok, "合成认证后必须到达 handler")
	apiKey := captured[0].(*service.APIKey)
	subject := captured[1].(AuthSubject)
	require.Zero(t, apiKey.ID)
	require.NotNil(t, apiKey.GroupID)
	require.Equal(t, int64(9), *apiKey.GroupID)
	require.NotNil(t, apiKey.Group)
	require.Equal(t, service.PlatformOpenAI, apiKey.Group.Platform)
	require.Zero(t, subject.UserID)
}

func TestCodexDiagnosticSyntheticAuthNegativeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth, err := service.NewCodexDiagnosticInternalAuth()
	require.NoError(t, err)
	marked := service.WithCodexDiagnosticGroup(service.WithCodexTicketDiagnostic(context.Background(), 41), 9)
	markedNoGroup := service.WithCodexTicketDiagnostic(context.Background(), 41)

	for _, test := range []struct {
		name  string
		ctx   context.Context
		token string
	}{
		// 仅 ctx 标记：令牌不匹配 → 走正常认证流程并按无效 Key 拒绝。
		{"marker only", marked, "wrong-token"},
		// 仅令牌：外部请求无法注入 ctx 标记 → 正常认证流程拒绝。
		{"token only", context.Background(), auth.Token()},
		// 标记 + 令牌但缺诊断分组 → 不合成，走正常认证流程拒绝。
		{"marker and token without group", markedNoGroup, auth.Token()},
		// 无标记无有效 Key。
		{"neither", context.Background(), "wrong-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			r, _ := newDiagnosticAuthTestRouter(t, auth, &calls)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, diagnosticRequest(test.ctx, test.token))
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Positive(t, calls.Load(), "负例必须落入正常 Key 查询流程")
		})
	}
}
