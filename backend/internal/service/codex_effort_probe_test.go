package service

import (
	"net/http"
	"testing"

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
