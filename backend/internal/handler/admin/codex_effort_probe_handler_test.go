package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeEffortProber struct {
	results map[string]service.CodexEffortProbeResult
}

func (f *fakeEffortProber) RunCodexEffortProbe(ctx context.Context, accountID int64, model string) service.CodexEffortProbeResult {
	return f.results[model]
}

func effortProbeRequest(t *testing.T, h *AccountHandler, accountID string, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/codex-effort-probe", h.EffortProbeCodexModels)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/codex-effort-probe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func newEffortProbeHandler(prober codexEffortProber, attempts *fakeCodexTicketAttemptRepo) *AccountHandler {
	h := &AccountHandler{}
	if prober != nil {
		h.SetCodexEffortProbeRunner(prober)
	}
	if attempts != nil {
		h.codexTicketAttempts = attempts
	}
	return h
}

// 判定映射的历史语义必须与指纹诊断一致：normal/degraded 是明确结论
// （outcome=success + matched true/false），uncertain 落 miss，failed 落 error。
// 历史视图按这套语义渲染降智判定，映射漂移会让两种检测的历史互相误读。
func TestEffortProbeCodexModelsRecordsHistory(t *testing.T) {
	prober := &fakeEffortProber{results: map[string]service.CodexEffortProbeResult{
		"gpt-6-astra": {Model: "gpt-6-astra", Status: "degraded", Reason: "effort_list_contains_none", HTTPStatus: 400, DurationMS: 640, SupportedEfforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
	}}
	attempts := &fakeCodexTicketAttemptRepo{}
	w := effortProbeRequest(t, newEffortProbeHandler(prober, attempts), "41", `{"models":["gpt-6-astra"]}`)
	require.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Data struct {
			Items []struct {
				Model            string   `json:"model"`
				Status           string   `json:"status"`
				Reason           string   `json:"reason"`
				HTTPStatus       int      `json:"http_status"`
				DurationMS       int      `json:"duration_ms"`
				SupportedEfforts []string `json:"supported_efforts"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "degraded", envelope.Data.Items[0].Status)
	require.Equal(t, 400, envelope.Data.Items[0].HTTPStatus)
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, envelope.Data.Items[0].SupportedEfforts)

	require.Len(t, attempts.inserted, 1)
	degraded := attempts.inserted[0]
	require.Equal(t, "effort_probe", degraded.VerificationMethod)
	require.Equal(t, "success", degraded.Outcome)
	require.NotNil(t, degraded.FingerprintMatched)
	require.False(t, *degraded.FingerprintMatched)
	require.Equal(t, "effort_list_contains_none", degraded.ReasonCode)
	require.NotNil(t, degraded.HTTPStatus)
	require.Equal(t, 400, *degraded.HTTPStatus)
}

func TestEffortProbeCodexModelsUncertainAndFailedOutcomes(t *testing.T) {
	prober := &fakeEffortProber{results: map[string]service.CodexEffortProbeResult{
		"gpt-6-astra": {Model: "gpt-6-astra", Status: "uncertain", Reason: "effort_list_missing", HTTPStatus: 400},
	}}
	attempts := &fakeCodexTicketAttemptRepo{}
	w := effortProbeRequest(t, newEffortProbeHandler(prober, attempts), "41", `{"models":["gpt-6-astra"]}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, attempts.inserted, 1)
	require.Equal(t, "miss", attempts.inserted[0].Outcome)
	require.Nil(t, attempts.inserted[0].FingerprintMatched)
}

// 探针判据只在 astra 系列上验证过：库内在列的非 astra 模型（如 gpt-6-sol）
// 与库外模型一样必须 400，不能放进探测流程得出未经验证的结论。
func TestEffortProbeCodexModelsValidation(t *testing.T) {
	prober := &fakeEffortProber{}
	// 模型不在指纹库白名单
	w := effortProbeRequest(t, newEffortProbeHandler(prober, nil), "41", `{"models":["gpt-4"]}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	// 库内但非 astra 系列
	w = effortProbeRequest(t, newEffortProbeHandler(prober, nil), "41", `{"models":["gpt-6-sol"]}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	// 未接线探针
	w = effortProbeRequest(t, newEffortProbeHandler(nil, nil), "41", `{"models":["gpt-6-astra"]}`)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
}
