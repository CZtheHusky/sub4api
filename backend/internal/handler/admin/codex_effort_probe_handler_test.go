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
		"gpt-6-sol":   {Model: "gpt-6-sol", Status: "degraded", Reason: "effort_list_contains_none", HTTPStatus: 400, DurationMS: 640, SupportedEfforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
		"gpt-6.1-sol": {Model: "gpt-6.1-sol", Status: "normal", DurationMS: 710, SupportedEfforts: []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
	}}
	attempts := &fakeCodexTicketAttemptRepo{}
	w := effortProbeRequest(t, newEffortProbeHandler(prober, attempts), "41", `{"models":["gpt-6-sol","gpt-6.1-sol"]}`)
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
	require.Len(t, envelope.Data.Items, 2)
	require.Equal(t, "degraded", envelope.Data.Items[0].Status)
	require.Equal(t, 400, envelope.Data.Items[0].HTTPStatus)
	require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, envelope.Data.Items[0].SupportedEfforts)
	require.Equal(t, "normal", envelope.Data.Items[1].Status)

	require.Len(t, attempts.inserted, 2)
	degraded := attempts.inserted[0]
	require.Equal(t, "effort_probe", degraded.VerificationMethod)
	require.Equal(t, "success", degraded.Outcome)
	require.NotNil(t, degraded.FingerprintMatched)
	require.False(t, *degraded.FingerprintMatched)
	require.Equal(t, "effort_list_contains_none", degraded.ReasonCode)
	require.NotNil(t, degraded.HTTPStatus)
	require.Equal(t, 400, *degraded.HTTPStatus)
	normal := attempts.inserted[1]
	require.Equal(t, "success", normal.Outcome)
	require.NotNil(t, normal.FingerprintMatched)
	require.True(t, *normal.FingerprintMatched)
	require.Empty(t, normal.ReasonCode)
}

func TestEffortProbeCodexModelsUncertainAndFailedOutcomes(t *testing.T) {
	prober := &fakeEffortProber{results: map[string]service.CodexEffortProbeResult{
		"gpt-6-sol": {Model: "gpt-6-sol", Status: "uncertain", Reason: "effort_list_missing", HTTPStatus: 400},
		"gpt-5.6-luna": {Model: "gpt-5.6-luna", Status: "failed", Reason: "unauthorized", HTTPStatus: 401, DurationMS: 120},
	}}
	attempts := &fakeCodexTicketAttemptRepo{}
	w := effortProbeRequest(t, newEffortProbeHandler(prober, attempts), "41", `{"models":["gpt-6-sol","gpt-5.6-luna"]}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, attempts.inserted, 2)
	require.Equal(t, "miss", attempts.inserted[0].Outcome)
	require.Nil(t, attempts.inserted[0].FingerprintMatched)
	require.Equal(t, "error", attempts.inserted[1].Outcome)
	require.Equal(t, "unauthorized", attempts.inserted[1].ReasonCode)
}

func TestEffortProbeCodexModelsValidation(t *testing.T) {
	prober := &fakeEffortProber{}
	// 模型不在指纹库白名单
	w := effortProbeRequest(t, newEffortProbeHandler(prober, nil), "41", `{"models":["gpt-4"]}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	// 未接线探针
	w = effortProbeRequest(t, newEffortProbeHandler(nil, nil), "41", `{"models":["gpt-6-sol"]}`)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
}
