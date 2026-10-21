//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWSProbeHandshake401UsesHTTPRefreshAndReplay(t *testing.T) {
	for _, finalStatus := range []int{400, 401} {
		t.Run(http.StatusText(finalStatus), func(t *testing.T) {
			svc, d, _, _ := newWSProbeService()
			account, repo, executor, provider := newOpenAIRecoveryFixture()
			svc.accountRepo, svc.openAITokenProvider = repo, provider
			d.err = errors.New("upgrade rejected")
			d.status = 401
			calls := 0
			svc.httpUpstream = &openAIRecoveryUpstream{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				calls++
				status := 401
				if calls == 2 {
					require.Equal(t, "Bearer repaired-access", req.Header.Get("Authorization"))
					status = finalStatus
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(wsProbeNormal))}, nil
			}}
			result := svc.RunCodexEffortProbe(context.Background(), account.ID, "gpt-6-astra")
			require.Equal(t, 1, d.calls)
			require.Equal(t, 2, calls)
			require.Equal(t, int32(1), executor.calls.Load())
			require.Equal(t, "http", result.Transport)
			if finalStatus == 400 {
				require.Equal(t, "normal", result.Status)
			} else {
				require.Equal(t, "failed", result.Status)
				require.Equal(t, "unauthorized", result.Reason)
			}
		})
	}
}
