//go:build unit

package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeGatewayDiagnosticRunner struct {
	output         string
	httpStatus     int
	gatewayErrCode string
	reason         string
	calls          int
}

func (f *fakeGatewayDiagnosticRunner) RunGatewayDiagnostic(ctx context.Context, accountID int64, model string, challenge ModelTraceChallenge, timeout time.Duration) (string, int, string, string) {
	f.calls++
	return f.output, f.httpStatus, f.gatewayErrCode, f.reason
}

type fakeAutoCheckAttemptRepo struct {
	CodexTicketAttemptRepository
	inserted []*CodexTicketAttempt
}

func (r *fakeAutoCheckAttemptRepo) Insert(ctx context.Context, a *CodexTicketAttempt) error {
	r.inserted = append(r.inserted, a)
	return nil
}

func autoCheckReferenceText(t *testing.T, model string) (string, int) {
	t.Helper()
	raw, err := os.ReadFile("testdata/modeltrace_gpt_reference.json")
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

func TestCodexAutoDegradationGatewayProbeVerdictAndAttempt(t *testing.T) {
	t.Run("scorable output yields verdict and auto_check attempt", func(t *testing.T) {
		text, _ := autoCheckReferenceText(t, "gpt-6-sol")
		runner := &fakeGatewayDiagnosticRunner{output: text, httpStatus: 200}
		attempts := &fakeAutoCheckAttemptRepo{}
		probe := &codexAutoDegradationGatewayProbe{runner: runner, attempts: attempts, timeout: time.Second}

		verdict, err := probe.ProbeCodexAutoDegradation(context.Background(), 41, "gpt-6-sol")
		require.NoError(t, err)
		require.Equal(t, codexAutoDegradationStatusNormal, verdict.Status)
		require.Equal(t, 1, runner.calls)
		require.Len(t, attempts.inserted, 1)
		attempt := attempts.inserted[0]
		require.Equal(t, "auto_check", attempt.Trigger)
		require.Equal(t, int64(41), attempt.AccountID)
		require.Equal(t, "gpt-6-sol", attempt.Model)
		require.Equal(t, "success", attempt.Outcome)
		require.NotNil(t, attempt.FingerprintMatched)
		require.True(t, *attempt.FingerprintMatched)
		require.Equal(t, "gpt-6-sol", attempt.FingerprintPredictedModel)
	})

	t.Run("degraded verdict persists unmatched fingerprint", func(t *testing.T) {
		text, _ := autoCheckReferenceText(t, "gpt-6-sol")
		runner := &fakeGatewayDiagnosticRunner{output: text, httpStatus: 200}
		attempts := &fakeAutoCheckAttemptRepo{}
		probe := &codexAutoDegradationGatewayProbe{runner: runner, attempts: attempts, timeout: time.Second}

		verdict, err := probe.ProbeCodexAutoDegradation(context.Background(), 41, "gpt-5.6-luna")
		require.NoError(t, err)
		require.Equal(t, codexAutoDegradationStatusDegraded, verdict.Status)
		require.Len(t, attempts.inserted, 1)
		require.NotNil(t, attempts.inserted[0].FingerprintMatched)
		require.False(t, *attempts.inserted[0].FingerprintMatched)
	})

	t.Run("runner failure maps to failed verdict without attempt", func(t *testing.T) {
		runner := &fakeGatewayDiagnosticRunner{reason: "no_group"}
		attempts := &fakeAutoCheckAttemptRepo{}
		probe := &codexAutoDegradationGatewayProbe{runner: runner, attempts: attempts, timeout: time.Second}

		verdict, err := probe.ProbeCodexAutoDegradation(context.Background(), 41, "gpt-6-sol")
		require.NoError(t, err)
		require.Equal(t, codexAutoDegradationStatusFailed, verdict.Status)
		require.Equal(t, "no_group", verdict.Reason)
		require.Empty(t, attempts.inserted, "未拿到可评分输出不落库")
	})

	t.Run("nil runner is unavailable", func(t *testing.T) {
		probe := &codexAutoDegradationGatewayProbe{}
		_, err := probe.ProbeCodexAutoDegradation(context.Background(), 41, "gpt-6-sol")
		require.Error(t, err)
	})
}
