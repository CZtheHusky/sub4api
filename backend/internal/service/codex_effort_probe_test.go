package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MACOS-DO/sub4api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// 报错体取自 2026-10-20 对真实降智账号的探针实测：400 的 Supported values
// 列表反映服务侧模型的档位目录——luna 系独有 none、不支持 ultra。
func TestClassifyCodexEffortProbeResponse(t *testing.T) {
	degradedBody := []byte(`{"error":{"message":"Unsupported value: 'minimal' is not supported with the 'gpt-6.1-sol' model. Supported values are: 'none', 'low', 'medium', 'high', 'xhigh', and 'max'.","type":"invalid_request_error","param":"reasoning.effort","code":"unsupported_value"}}`)
	normalBody := []byte(`{"error":{"message":"Unsupported value: 'minimal' is not supported with the 'gpt-6.1-sol' model. Supported values are: 'low', 'medium', 'high', 'xhigh', 'max', and 'ultra'.","type":"invalid_request_error","param":"reasoning.effort","code":"unsupported_value"}}`)

	status, reason, efforts := classifyCodexEffortProbeResponse(http.StatusBadRequest, degradedBody)
	require.Equal(t, "degraded", status)
	require.Equal(t, "effort_list_contains_none", reason)
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, efforts)

	status, reason, efforts = classifyCodexEffortProbeResponse(http.StatusBadRequest, normalBody)
	require.Equal(t, "normal", status)
	require.Empty(t, reason)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, efforts)

	status, reason, efforts = classifyCodexEffortProbeResponse(http.StatusBadRequest, []byte(`{"error":{"message":"bad request"}}`))
	require.Equal(t, "uncertain", status)
	require.Equal(t, "effort_list_missing", reason)
	require.Nil(t, efforts)

	status, reason, _ = classifyCodexEffortProbeResponse(http.StatusOK, nil)
	require.Equal(t, "uncertain", status)
	require.Equal(t, "minimal_accepted", reason)

	status, reason, _ = classifyCodexEffortProbeResponse(http.StatusUnauthorized, nil)
	require.Equal(t, "failed", status)
	require.Equal(t, "unauthorized", reason)

	status, reason, _ = classifyCodexEffortProbeResponse(http.StatusInternalServerError, nil)
	require.Equal(t, "failed", status)
	require.Equal(t, "upstream_error", reason)
}

type effortProbeUpstream struct {
	req      *http.Request
	proxyURL string
	resp     *http.Response
}

func (u *effortProbeUpstream) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.req, u.proxyURL = req, proxyURL
	return u.resp, nil
}

func (u *effortProbeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

type effortProbeAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *effortProbeAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

// 探针必须走生产传输栈（httpUpstream + OpenAI profile），不能另开裸 client：
// 线上靠插件/传输策略伪装 TLS 指纹时，探针若以 Go 原生指纹出站，就是全链路
// 保真体系里唯一的破例流量。
func TestRunCodexEffortProbeUsesProductionUpstreamStack(t *testing.T) {
	degradedBody := `{"error":{"message":"Unsupported value: 'minimal' is not supported with the 'gpt-6.1-sol' model. Supported values are: 'none', 'low', 'medium', 'high', 'xhigh', and 'max'.","type":"invalid_request_error","param":"reasoning.effort","code":"unsupported_value"}}`
	upstream := &effortProbeUpstream{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(degradedBody)),
	}}
	proxyID := int64(7)
	account := &Account{
		ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "tok"},
		ProxyID:     &proxyID,
		Proxy:       &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 7890},
	}
	svc := &OpenAIGatewayService{accountRepo: &effortProbeAccountRepo{account: account}, httpUpstream: upstream}

	result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6.1-sol")
	require.Equal(t, "degraded", result.Status)
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, result.SupportedEfforts)

	require.NotNil(t, upstream.req)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.req.Context()))
	require.Equal(t, chatgptCodexURL, upstream.req.URL.String())
	require.Equal(t, "http://127.0.0.1:7890", upstream.proxyURL)
	require.Equal(t, "Bearer tok", upstream.req.Header.Get("Authorization"))
	body, err := io.ReadAll(upstream.req.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"effort":"minimal"`)
	require.Contains(t, string(body), `"model":"gpt-6.1-sol"`)
}
