//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MACOS-DO/sub4api/internal/service"

	"github.com/stretchr/testify/require"
)

func TestGroupModelAllowlistSkipsCodexDiagnostic(t *testing.T) {
	// 白名单开启且模型不在名单内：普通请求 404，降智探测（ctx 标记）放行。
	apiKey := allowlistAPIKey(true, "claude-sonnet-4.5")
	router, calls := newGroupModelAllowlistTestRouter(apiKey, "/v1")

	body := `{"model":"claude-not-allowed","input":"x"}`
	normal := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, normal)
	require.Equal(t, http.StatusNotFound, w.Code)

	diagnostic := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	diagnostic = diagnostic.WithContext(service.WithCodexTicketDiagnostic(context.Background(), 41))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, diagnostic)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, *calls, "handler:/v1/responses")
}
