//go:build integration

package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/repository"
	"github.com/MACOS-DO/sub4api/internal/service"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestWSProbeEndpointDatabaseSmoke(t *testing.T) {
	dsn := os.Getenv("CODEX_WS_PROBE_DSN")
	if dsn == "" {
		t.Skip("CODEX_WS_PROBE_DSN requires an isolated PostgreSQL test database")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo := repository.NewCodexTicketAttemptRepository(db)
	accountID := time.Now().UnixNano()
	t.Cleanup(func() { _, err := repo.Delete(context.Background(), accountID, "", ""); require.NoError(t, err) })
	enabled := false
	length := 780
	prober := &fakeEffortProber{results: map[string]service.CodexEffortProbeResult{"gpt-6-astra": {Model: "gpt-6-astra", Status: "normal", HTTPStatus: 400, Transport: "ws", SafetyBufferingEnabled: &enabled, SafetyBufferingFasterModel: "test-marker", TurnStateLen: &length}}}
	h := &AccountHandler{codexTicketAttempts: repo, codexEffortProber: prober}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/codex-effort-probe", h.EffortProbeCodexModels)
	router.GET("/accounts/:id/codex-ticket-history", h.GetCodexTicketHistory)
	path := fmt.Sprintf("/accounts/%d", accountID)
	post := httptest.NewRequest(http.MethodPost, path+"/codex-effort-probe", strings.NewReader(`{"models":["gpt-6-astra"]}`))
	post.Header.Set("Content-Type", "application/json")
	immediate := httptest.NewRecorder()
	router.ServeHTTP(immediate, post)
	require.Equal(t, 200, immediate.Code)
	history := httptest.NewRecorder()
	router.ServeHTTP(history, httptest.NewRequest(http.MethodGet, path+"/codex-ticket-history?page=1&page_size=20", nil))
	require.Equal(t, 200, history.Code)
	var envelope struct {
		Data struct {
			Items []service.CodexTicketAttempt `json:"items"`
			Total int                          `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &envelope))
	require.Equal(t, 1, envelope.Data.Total)
	require.Len(t, envelope.Data.Items, 1)
	row := envelope.Data.Items[0]
	require.Equal(t, "ws", row.Transport)
	require.NotNil(t, row.SafetyBufferingEnabled)
	require.False(t, *row.SafetyBufferingEnabled)
	require.Equal(t, "test-marker", row.SafetyBufferingFasterModel)
	require.Equal(t, 780, *row.TurnStateLen)
	require.Equal(t, "effort_probe", row.VerificationMethod)
	// Persist the real history response for the ccvps browser-component smoke test.
	if output := os.Getenv("CODEX_WS_PROBE_HISTORY_FIXTURE"); output != "" {
		require.NoError(t, os.WriteFile(output, history.Body.Bytes(), 0644))
	}
}
