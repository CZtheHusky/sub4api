package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const wsProbeNormal = `{"type":"error","error":{"type":"invalid_request_error","code":"unsupported_value","message":"Unsupported value: 'minimal'. Supported values are: 'low', 'medium', and 'high'."}}`
const wsProbeDegraded = `{"type":"error","error":{"type":"invalid_request_error","message":"Unsupported value: 'minimal'. Supported values are: 'none', 'low', and 'high'."}}`
const wsProbeMetadata = `{"type":"codex.response.metadata","headers":{"x-codex-safety-buffering-enabled":"true","x-codex-safety-buffering-faster-model":"gpt-6-luna","x-codex-turn-state":"opaque"}}`

type wsProbeConn struct {
	frames   []string
	payload  []byte
	closed   bool
	reads    int
	read     func(context.Context) ([]byte, error)
	writeErr error
}

func (c *wsProbeConn) WriteJSON(_ context.Context, v any) error {
	c.payload, _ = json.Marshal(v)
	return c.writeErr
}
func (c *wsProbeConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.reads++
	if len(c.frames) > 0 {
		frame := c.frames[0]
		c.frames = c.frames[1:]
		return []byte(frame), nil
	}
	if c.read != nil {
		return c.read(ctx)
	}
	return nil, io.EOF
}
func (c *wsProbeConn) Ping(context.Context) error { return nil }
func (c *wsProbeConn) Close() error               { c.closed = true; return nil }

type wsProbeDialer struct {
	conn       *wsProbeConn
	calls      int
	headers    http.Header
	proxy, url string
	err        error
	status     int
}

func (d *wsProbeDialer) Dial(_ context.Context, url string, h http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	d.calls++
	d.headers, d.proxy, d.url = h.Clone(), proxy, url
	return d.conn, d.status, nil, d.err
}
func newWSProbeService(frames ...string) (*OpenAIGatewayService, *wsProbeDialer, *effortProbeUpstream, *Account) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled, cfg.Gateway.OpenAIWS.OAuthEnabled, cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true, true, true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "tok", "chatgpt_account_id": "acct-41"},
		Extra:       map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool}}
	d := &wsProbeDialer{conn: &wsProbeConn{frames: frames}}
	upstream := &effortProbeUpstream{resp: &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(wsProbeNormal))}}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: &effortProbeAccountRepo{account: account}, httpUpstream: upstream, openaiWSPassthroughDialer: d}
	return svc, d, upstream, account
}

func TestWSProbeVerdictAndMetadataOrdering(t *testing.T) {
	for _, late := range []bool{false, true} {
		for _, degraded := range []bool{false, true} {
			t.Run(fmt.Sprintf("late=%v/degraded=%v", late, degraded), func(t *testing.T) {
				terminal := wsProbeNormal
				expected := "normal"
				if degraded {
					terminal, expected = wsProbeDegraded, "degraded"
				}
				frames := []string{wsProbeMetadata, terminal}
				if late {
					frames = []string{terminal, wsProbeMetadata}
				}
				frames = append([]string{`{"type":"codex.rate_limits"}`, `{"type":"unknown"}`, `{"type":"unknown"}`, `{"type":"unknown"}`, `{"type":"unknown"}`}, frames...)
				svc, d, httpUpstream, account := newWSProbeService(frames...)
				proxyID := int64(3)
				account.ProxyID = &proxyID
				account.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 7890}
				result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
				require.Equal(t, expected, result.Status)
				require.Equal(t, 400, result.HTTPStatus)
				require.Equal(t, "ws", result.Transport)
				require.NotNil(t, result.SafetyBufferingEnabled)
				require.True(t, *result.SafetyBufferingEnabled)
				require.Equal(t, "gpt-6-luna", result.SafetyBufferingFasterModel)
				require.Equal(t, 6, *result.TurnStateLen)
				require.True(t, d.conn.closed)
				require.Nil(t, httpUpstream.req)
				require.Nil(t, svc.openaiWSPool)
				require.Equal(t, "http://127.0.0.1:7890", d.proxy)
				require.Equal(t, "wss://chatgpt.com/backend-api/codex/responses", d.url)
				require.Equal(t, "response.create", gjson.GetBytes(d.conn.payload, "type").String())
				require.Equal(t, int64(16), gjson.GetBytes(d.conn.payload, "max_output_tokens").Int())
				require.False(t, gjson.GetBytes(d.conn.payload, "previous_response_id").Exists())
				require.Empty(t, d.headers.Get(openAIWSTurnStateHeader))
				session := gjson.GetBytes(d.conn.payload, "client_metadata.session_id").String()
				require.Equal(t, session, d.headers.Get("session-id"))
				require.Equal(t, gjson.GetBytes(d.conn.payload, "client_metadata.thread_id").String(), d.headers.Get("thread-id"))
				require.Equal(t, session, gjson.GetBytes(d.conn.payload, "prompt_cache_key").String())
			})
		}
	}
}

func TestWSProbeMissingMetadataAndDrainLimits(t *testing.T) {
	for _, mode := range []string{"close", "timeout", "frame_limit", "empty_metadata", "false"} {
		t.Run(mode, func(t *testing.T) {
			svc, d, u, _ := newWSProbeService(wsProbeNormal)
			switch mode {
			case "timeout":
				d.conn.read = func(ctx context.Context) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
			case "frame_limit":
				for i := 0; i < 20; i++ {
					d.conn.frames = append(d.conn.frames, `{"type":"unknown"}`)
				}
			case "empty_metadata":
				d.conn.frames = append(d.conn.frames, `{"type":"codex.response.metadata","headers":{}}`)
			case "false":
				d.conn.frames = append(d.conn.frames, `{"type":"codex.response.metadata","headers":{"x-codex-safety-buffering-enabled":"false","x-codex-turn-state":""}}`)
			}
			start := time.Now()
			result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
			require.Equal(t, "normal", result.Status)
			require.Nil(t, u.req)
			require.Less(t, time.Since(start), time.Second)
			require.LessOrEqual(t, d.conn.reads, 9)
			if mode == "false" {
				require.NotNil(t, result.SafetyBufferingEnabled)
				require.False(t, *result.SafetyBufferingEnabled)
				require.Equal(t, 0, *result.TurnStateLen)
			} else {
				require.Nil(t, result.SafetyBufferingEnabled)
				require.Nil(t, result.TurnStateLen)
			}
		})
	}
}

func TestWSProbeTerminalFailuresAndAcceptedNeverFallback(t *testing.T) {
	for _, tc := range []struct {
		frame, status, reason string
		http                  int
	}{
		{`{"type":"error","error":{"type":"authentication_error"}}`, "failed", "unauthorized", 401},
		{`{"type":"error","error":{"type":"permission_error"}}`, "failed", "unauthorized", 403},
		{`{"type":"error","error":{"type":"rate_limit_error"}}`, "failed", "upstream_error", 429},
		{`{"type":"error","error":{"type":"server_error"}}`, "failed", "upstream_error", 502},
		{`{"type":"error","error":{"type":"invalid_request_error"}}`, "uncertain", "effort_list_missing", 400},
		{`{"type":"response.failed","response":{"error":{"type":"authentication_error"}}}`, "failed", "unauthorized", 401},
		{`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded"}}}`, "failed", "upstream_error", 429},
		{`{"type":"response.failed","response":{"error":{"type":"server_error"}}}`, "failed", "upstream_error", 502},
		{`{"type":"response.cancelled"}`, "failed", "upstream_error", 502},
		{`{"type":"response.created"}`, "uncertain", "minimal_accepted", 200},
		{`{"type":"response.in_progress"}`, "uncertain", "minimal_accepted", 200},
		{`{"type":"response.output_text.delta"}`, "uncertain", "minimal_accepted", 200},
		{`{"type":"response.completed"}`, "uncertain", "minimal_accepted", 200},
	} {
		t.Run(tc.frame, func(t *testing.T) {
			svc, d, u, _ := newWSProbeService(wsProbeMetadata, tc.frame)
			result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
			require.Equal(t, tc.status, result.Status)
			require.Equal(t, tc.reason, result.Reason)
			require.Equal(t, tc.http, result.HTTPStatus)
			require.Nil(t, u.req)
			require.True(t, d.conn.closed)
			require.Equal(t, 2, d.conn.reads)
		})
	}
}

func TestWSProbeFallbackClearsMetadataAndClosesFirst(t *testing.T) {
	for _, stage := range []string{"handshake401", "handshake403", "write", "read"} {
		t.Run(stage, func(t *testing.T) {
			svc, d, u, _ := newWSProbeService(wsProbeMetadata)
			if strings.HasPrefix(stage, "handshake") {
				d.err = errors.New("upgrade failed")
				d.status = 401
				if stage == "handshake403" {
					d.status = 403
				}
			}
			if stage == "write" {
				d.conn.writeErr = io.ErrClosedPipe
			}
			u.beforeDo = func() {
				if d.err == nil {
					require.True(t, d.conn.closed, "close WS before HTTP fallback")
				}
			}
			result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
			require.Equal(t, "normal", result.Status)
			require.Equal(t, "http", result.Transport)
			require.Equal(t, 1, d.calls)
			require.Nil(t, result.SafetyBufferingEnabled)
			require.Empty(t, result.SafetyBufferingFasterModel)
			require.Nil(t, result.TurnStateLen)
			require.Equal(t, 1, u.calls)
			require.NotNil(t, u.req)
			if d.err == nil {
				require.True(t, d.conn.closed)
			}
			body, err := io.ReadAll(u.req.Body)
			require.NoError(t, err)
			require.False(t, gjson.GetBytes(body, "client_metadata").Exists())
			require.False(t, gjson.GetBytes(body, "type").Exists())
		})
	}
}

func TestWSProbeTimeoutGetsFreshHTTPBudget(t *testing.T) {
	svc, d, u, _ := newWSProbeService()
	d.conn.read = func(ctx context.Context) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
	result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
	require.Equal(t, "normal", result.Status)
	require.Equal(t, "http", result.Transport)
	deadline, ok := u.req.Context().Deadline()
	require.True(t, ok)
	require.Greater(t, time.Until(deadline), 14*time.Second)
	require.GreaterOrEqual(t, result.DurationMS, 15000)
}

func TestWSProbeCallerCancellationNeverFallback(t *testing.T) {
	svc, d, u, _ := newWSProbeService()
	ctx, cancel := context.WithCancel(context.Background())
	d.conn.read = func(context.Context) ([]byte, error) { cancel(); return nil, context.Canceled }
	result := svc.RunCodexEffortProbe(ctx, 41, "gpt-6-astra")
	require.Equal(t, "failed", result.Status)
	require.True(t, d.conn.closed)
	require.Nil(t, u.req)
}

func TestWSProbeProtocolAndPluginRouting(t *testing.T) {
	for _, mode := range []string{"off", "http_bridge", "v1", "disabled", "force_http", "plugin_unavailable"} {
		t.Run(mode, func(t *testing.T) {
			svc, d, u, a := newWSProbeService(wsProbeNormal)
			switch mode {
			case "off", "http_bridge":
				a.Extra["openai_oauth_responses_websockets_v2_mode"] = mode
			case "v1":
				svc.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = false
				svc.cfg.Gateway.OpenAIWS.ResponsesWebsockets = true
			case "disabled":
				svc.cfg.Gateway.OpenAIWS.Enabled = false
			case "force_http":
				svc.cfg.Gateway.OpenAIWS.ForceHTTP = true
			case "plugin_unavailable":
				svc.pluginManager = &PluginManager{}
				svc.pluginManager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: 100, unavailable: "offline"})
			}
			result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
			require.Zero(t, d.calls)
			require.Equal(t, "http", result.Transport)
			require.Nil(t, result.SafetyBufferingEnabled)
			if mode == "plugin_unavailable" {
				require.Equal(t, "failed", result.Status)
				require.Nil(t, u.req)
			} else {
				require.Equal(t, "normal", result.Status)
				require.NotNil(t, u.req)
			}
		})
	}
}

func TestWSProbeProductionHandshakeIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := codexIdentityEnforcement.Load()
	t.Cleanup(func() { codexIdentityEnforcement.Store(old) })
	for _, enforced := range []bool{true, false} {
		for _, ua := range []string{"", "codex_vscode/0.150.0 (Ubuntu 22.4.0; x86_64) vscode"} {
			for _, shadow := range []bool{false, true} {
				svc, _, _, a := newWSProbeService()
				codexIdentityEnforcement.Store(enforced)
				a.Credentials["user_agent"] = ua
				source := a
				if shadow {
					copy := *a
					source = &copy
					parentID := int64(51)
					source.ID = parentID
					a.ParentAccountID = &parentID
					svc.accountRepo = &wsProbeIdentityRepo{selected: a, source: source}
				}
				payload := map[string]any{"model": "gpt-6-astra"}
				_, probeHeaders, err := svc.buildCodexEffortProbeWSRequest(context.Background(), a, "tok", payload)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Request.Header.Set("session-id", codexEffortProbeSession)
				c.Request.Header.Set("thread-id", codexEffortProbeSession)
				c.Request.Header.Set("User-Agent", resolveCodexOutboundIdentity(svc.codexIdentityOverrideUA(a)).userAgent)
				resolved, err := svc.prepareCodexAccountIdentitySource(context.Background(), c, a)
				require.NoError(t, err)
				require.Equal(t, source, resolved)
				productionPayload := map[string]any{"model": "gpt-6-astra", "prompt_cache_key": codexEffortProbeSession, "client_metadata": map[string]any{"session_id": codexEffortProbeSession, "thread_id": codexEffortProbeSession}}
				applyCodexAccountIdentityClientMetadataMap(productionPayload, resolved, 0)
				h, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, a, "tok", svc.getOpenAIWSProtocolResolver().Resolve(a), true, "", "", codexEffortProbeSession, "gpt-6-astra", "")
				require.NoError(t, err)
				require.NoError(t, normalizeCodexRequestMetadataMap(productionPayload, h, a))
				projectCodexOutboundHeaders(h, true)
				for _, key := range []string{"authorization", "originator", "version", "User-Agent", "OpenAI-Beta", "chatgpt-account-id", "session-id", "thread-id", "x-codex-beta-features"} {
					require.Equal(t, h.Values(key), probeHeaders.Values(key), "%s enforced=%v ua=%q shadow=%v", key, enforced, ua, shadow)
				}
				require.Empty(t, probeHeaders.Get("session_id"))
				require.Empty(t, probeHeaders.Get("conversation_id"))
				firstSession := probeHeaders.Get("session-id")
				_, again, err := svc.buildCodexEffortProbeWSRequest(context.Background(), a, "tok", map[string]any{})
				require.NoError(t, err)
				require.Equal(t, firstSession, again.Get("session-id"))
				source.Credentials = map[string]any{"chatgpt_account_id": "other-account", "access_token": "tok"}
				_, other, err := svc.buildCodexEffortProbeWSRequest(context.Background(), a, "tok", map[string]any{})
				require.NoError(t, err)
				require.NotEqual(t, firstSession, other.Get("session-id"))
			}
		}
	}
}

type wsProbeIdentityRepo struct {
	AccountRepository
	selected, source *Account
}

func (r *wsProbeIdentityRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if id == r.selected.ID {
		return r.selected, nil
	}
	return r.source, nil
}

func TestWSProbeMultipleModelsKeepTransportObservations(t *testing.T) {
	for _, httpFirst := range []bool{true, false} {
		svc, d, u, _ := newWSProbeService()
		results := make([]CodexEffortProbeResult, 0, 2)
		for index := 0; index < 2; index++ {
			isHTTP := (index == 0) == httpFirst
			d.err = nil
			d.conn = &wsProbeConn{frames: []string{wsProbeMetadata, wsProbeDegraded}}
			if isHTTP {
				d.err = io.EOF
			}
			result := svc.RunCodexEffortProbe(context.Background(), 41, fmt.Sprintf("astra-%d", index))
			results = append(results, result)
		}
		require.Equal(t, 2, d.calls)
		require.Equal(t, 1, u.calls)
		for index, result := range results {
			if (index == 0) == httpFirst {
				require.Equal(t, "http", result.Transport)
				require.Equal(t, "normal", result.Status)
				require.Nil(t, result.SafetyBufferingEnabled)
				require.Nil(t, result.TurnStateLen)
			} else {
				require.Equal(t, "ws", result.Transport)
				require.Equal(t, "degraded", result.Status)
				require.True(t, *result.SafetyBufferingEnabled)
				require.Equal(t, 6, *result.TurnStateLen)
			}
		}
	}
}

func TestWSProbeDrainRespectsCallerDeadlineAndKeepsVerdict(t *testing.T) {
	svc, d, u, _ := newWSProbeService(wsProbeNormal)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	callerDeadline, _ := ctx.Deadline()
	d.conn.read = func(readCtx context.Context) ([]byte, error) {
		deadline, ok := readCtx.Deadline()
		require.True(t, ok)
		require.False(t, deadline.After(callerDeadline))
		<-readCtx.Done()
		return nil, readCtx.Err()
	}
	result := svc.RunCodexEffortProbe(ctx, 41, "gpt-6-astra")
	require.Equal(t, "normal", result.Status)
	require.Nil(t, u.req)
	require.True(t, d.conn.closed)
}

func TestWSProbeMetadataSurvivesFinalFailure(t *testing.T) {
	svc, _, u, _ := newWSProbeService(wsProbeMetadata, `{"type":"response.failed","response":{"error":{"type":"server_error"}}}`)
	result := svc.RunCodexEffortProbe(context.Background(), 41, "gpt-6-astra")
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "ws", result.Transport)
	require.True(t, *result.SafetyBufferingEnabled)
	require.Equal(t, 6, *result.TurnStateLen)
	require.Nil(t, u.req)
}
