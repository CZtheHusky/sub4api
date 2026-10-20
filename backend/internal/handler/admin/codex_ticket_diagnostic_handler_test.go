package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type diagnosticRunnerResult struct {
	output         string
	httpStatus     int
	gatewayErrCode string
	reason         string
}

type fakeDiagnosticRunner struct {
	results map[string]diagnosticRunnerResult
	calls   []int64
}

func (f *fakeDiagnosticRunner) RunGatewayDiagnostic(ctx context.Context, accountID int64, model string, challenge service.ModelTraceChallenge, timeout time.Duration) (string, int, string, string) {
	f.calls = append(f.calls, accountID)
	result := f.results[model]
	return result.output, result.httpStatus, result.gatewayErrCode, result.reason
}

type fakeCodexTicketAttemptRepo struct {
	service.CodexTicketAttemptRepository
	inserted []*service.CodexTicketAttempt
}

func (r *fakeCodexTicketAttemptRepo) Insert(ctx context.Context, a *service.CodexTicketAttempt) error {
	r.inserted = append(r.inserted, a)
	return nil
}

func diagnosticReferenceText(t *testing.T, model string) (string, int) {
	t.Helper()
	raw, err := os.ReadFile("../../service/testdata/modeltrace_gpt_reference.json")
	require.NoError(t, err)
	var samples []struct {
		Model         string `json:"model"`
		ExpectedCount int    `json:"expected_count"`
		Text          string `json:"text"`
	}
	require.NoError(t, json.Unmarshal(raw, &samples))
	for _, sample := range samples {
		if sample.Model == model {
			return sample.Text, sample.ExpectedCount
		}
	}
	t.Fatalf("no reference sample for %s", model)
	return "", 0
}

func diagnoseCodexModelsRequest(t *testing.T, h *AccountHandler, accountID string, body string, challenge service.ModelTraceChallenge) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/codex-ticket-diagnostic", func(c *gin.Context) {
		h.diagnoseCodexModels(c, func() (service.ModelTraceChallenge, error) { return challenge, nil })
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/codex-ticket-diagnostic", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func parseDiagnosticResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	return envelope.Data
}

func TestDiagnoseCodexModelsVerdictsAndAttemptPersistence(t *testing.T) {
	text, expectedCount := diagnosticReferenceText(t, "gpt-6-sol")
	challenge := service.ModelTraceChallenge{Prompt: "test", ExpectedCount: expectedCount}

	t.Run("three-state verdicts with attempt records", func(t *testing.T) {
		runner := &fakeDiagnosticRunner{results: map[string]diagnosticRunnerResult{
			"gpt-6-sol":    {output: text, httpStatus: 200},
			"gpt-6.1-sol":  {output: text, httpStatus: 200},
			"gpt-5.6-luna": {output: text, httpStatus: 200},
		}}
		attempts := &fakeCodexTicketAttemptRepo{}
		h := &AccountHandler{}
		h.SetCodexDiagnosticDependencies(runner, attempts, nil)

		w := diagnoseCodexModelsRequest(t, h, "41", `{"models":["gpt-6-sol","gpt-6.1-sol","gpt-5.6-luna"]}`, challenge)
		require.Equal(t, http.StatusOK, w.Code)
		data := parseDiagnosticResponse(t, w)
		rawItems, ok := data["items"].([]any)
		require.True(t, ok)
		require.Len(t, rawItems, 3)
		statuses := map[string]map[string]any{}
		for _, raw := range rawItems {
			item, ok := raw.(map[string]any)
			require.True(t, ok)
			model, ok := item["model"].(string)
			require.True(t, ok)
			statuses[model] = item
		}
		require.Equal(t, "normal", statuses["gpt-6-sol"]["status"])
		require.Equal(t, "uncertain", statuses["gpt-6.1-sol"]["status"])
		require.Equal(t, "reference_class_unavailable", statuses["gpt-6.1-sol"]["reason"])
		require.Equal(t, "degraded", statuses["gpt-5.6-luna"]["status"])
		require.Equal(t, "fingerprint_mismatch", statuses["gpt-5.6-luna"]["reason"])

		require.Len(t, attempts.inserted, 3)
		byModel := map[string]*service.CodexTicketAttempt{}
		for _, attempt := range attempts.inserted {
			require.Equal(t, "diagnostic", attempt.Trigger)
			require.Equal(t, int64(41), attempt.AccountID)
			require.Equal(t, "success", attempt.Outcome)
			byModel[attempt.Model] = attempt
		}
		require.NotNil(t, byModel["gpt-6-sol"].FingerprintMatched)
		require.True(t, *byModel["gpt-6-sol"].FingerprintMatched)
		require.Nil(t, byModel["gpt-6.1-sol"].FingerprintMatched, "uncertain 不得写匹配结论")
		require.Equal(t, "reference_class_unavailable", byModel["gpt-6.1-sol"].ReasonCode)
		require.NotNil(t, byModel["gpt-5.6-luna"].FingerprintMatched)
		require.False(t, *byModel["gpt-5.6-luna"].FingerprintMatched)
	})

	t.Run("gateway http failure records error attempt with gateway_error_code", func(t *testing.T) {
		runner := &fakeDiagnosticRunner{results: map[string]diagnosticRunnerResult{
			"gpt-6-sol": {httpStatus: 503, gatewayErrCode: "no_available_accounts", reason: "http_status"},
		}}
		attempts := &fakeCodexTicketAttemptRepo{}
		h := &AccountHandler{}
		h.SetCodexDiagnosticDependencies(runner, attempts, nil)

		w := diagnoseCodexModelsRequest(t, h, "41", `{"models":["gpt-6-sol"]}`, challenge)
		require.Equal(t, http.StatusOK, w.Code)
		data := parseDiagnosticResponse(t, w)
		items := data["items"].([]any)
		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "failed", item["status"])
		require.Equal(t, "http_status", item["reason"])
		require.Equal(t, "no_available_accounts", item["gateway_error_code"])

		require.Len(t, attempts.inserted, 1)
		require.Equal(t, "error", attempts.inserted[0].Outcome)
		require.Equal(t, 503, *attempts.inserted[0].HTTPStatus)
	})

	t.Run("preflight failure is not persisted", func(t *testing.T) {
		runner := &fakeDiagnosticRunner{results: map[string]diagnosticRunnerResult{
			"gpt-6-sol": {reason: "no_group"},
		}}
		attempts := &fakeCodexTicketAttemptRepo{}
		h := &AccountHandler{}
		h.SetCodexDiagnosticDependencies(runner, attempts, nil)

		w := diagnoseCodexModelsRequest(t, h, "41", `{"models":["gpt-6-sol"]}`, challenge)
		require.Equal(t, http.StatusOK, w.Code)
		data := parseDiagnosticResponse(t, w)
		items := data["items"].([]any)
		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "failed", item["status"])
		require.Equal(t, "no_group", item["reason"])
		require.Empty(t, attempts.inserted, "预检失败不落库")
	})

	t.Run("invalid and duplicate models rejected", func(t *testing.T) {
		h := &AccountHandler{}
		h.SetCodexDiagnosticDependencies(&fakeDiagnosticRunner{}, &fakeCodexTicketAttemptRepo{}, nil)
		for _, body := range []string{
			`{"models":["not-a-model"]}`,
			`{"models":["gpt-6-sol","gpt-6-sol"]}`,
			`{"models":[]}`,
		} {
			w := diagnoseCodexModelsRequest(t, h, "41", body, challenge)
			require.Equal(t, http.StatusBadRequest, w.Code, body)
		}
	})

	t.Run("runner unavailable", func(t *testing.T) {
		h := &AccountHandler{}
		w := diagnoseCodexModelsRequest(t, h, "41", `{"models":["gpt-6-sol"]}`, challenge)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("invalid account id", func(t *testing.T) {
		h := &AccountHandler{}
		h.SetCodexDiagnosticDependencies(&fakeDiagnosticRunner{}, &fakeCodexTicketAttemptRepo{}, nil)
		w := diagnoseCodexModelsRequest(t, h, "abc", `{"models":["gpt-6-sol"]}`, challenge)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestCodexTicketHistoryHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	attempts := &fakeCodexTicketAttemptRepo{}
	h := &AccountHandler{}
	h.SetCodexDiagnosticDependencies(nil, attempts, nil)

	router := gin.New()
	router.GET("/accounts/:id/codex-ticket-history", h.GetCodexTicketHistory)
	router.DELETE("/accounts/:id/codex-ticket-history", h.DeleteCodexTicketHistory)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/accounts/41/codex-ticket-history?filter=bogus", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/accounts/41/codex-ticket-history?filter=bogus", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
