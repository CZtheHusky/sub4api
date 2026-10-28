package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSConnPool_LifetimeAdmission(t *testing.T) {
	for _, path := range []string{"ordinary", "preferred", "routing", "strict"} {
		for _, age := range []time.Duration{54 * time.Minute, 55 * time.Minute, 59 * time.Minute, 60 * time.Minute} {
			t.Run(path+"/"+age.String(), func(t *testing.T) {
				cfg := &config.Config{}
				cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
				cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
				pool := newOpenAIWSConnPool(cfg)
				t.Cleanup(pool.Close)
				pool.setClientDialerForTest(&openAIWSFakeDialer{})
				req := openAIWSAcquireRequest{Account: &Account{ID: 991, Type: AccountTypeAPIKey}, WSURL: "wss://example.com/responses"}
				if path == "routing" {
					req.Headers = http.Header{"X-Codex-Routing-Hint": []string{"route"}}
				}
				lease, err := pool.Acquire(context.Background(), req)
				require.NoError(t, err)
				old := lease.conn
				old.createdAtNano.Store(time.Now().Add(-age).UnixNano())
				lease.Release()
				if path == "preferred" || path == "strict" {
					req.PreferredConnID = old.id
				}
				req.ForcePreferredConn = path == "strict"
				next, err := pool.Acquire(context.Background(), req)
				if path == "strict" && age >= time.Hour {
					require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable, "hard-expired continuation must not send a new create")
					return
				}
				require.NoError(t, err)
				defer next.Release()
				if age < 55*time.Minute || path == "strict" {
					require.Same(t, old, next.conn, "strict continuation retains its only context until the hard limit")
				} else {
					require.NotSame(t, old, next.conn, "migratable requests must reserve the five minute lifetime margin")
				}
			})
		}
	}
}

func TestOpenAIWSConnPool_LifetimeExactBoundaries(t *testing.T) {
	now := time.Now()
	conn := newOpenAIWSConn("boundary", 1, nil, nil)
	for _, age := range []time.Duration{openAIWSConnRotateAge - 1, openAIWSConnRotateAge, openAIWSConnRotateAge + 1, openAIWSConnMaxAge - 1, openAIWSConnMaxAge} {
		conn.createdAtNano.Store(now.Add(-age).UnixNano())
		require.Equal(t, age < openAIWSConnRotateAge, conn.canStartTurn(now, false), age.String())
		require.Equal(t, age < openAIWSConnMaxAge, conn.canStartTurn(now, true), age.String())
	}
}

func TestOpenAIWSConnPool_LifetimeQueueCrossing(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprint(strict), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			pool.setClientDialerForTest(&openAIWSFakeDialer{})
			req := openAIWSAcquireRequest{Account: &Account{ID: 992, Type: AccountTypeAPIKey}, WSURL: "wss://example.com/responses"}
			first, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			req.PreferredConnID = first.ConnID()
			req.ForcePreferredConn = strict
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			type acquired struct {
				lease *openAIWSConnLease
				err   error
			}
			done := make(chan acquired, 1)
			go func() { lease, err := pool.Acquire(ctx, req); done <- acquired{lease, err} }()
			require.Eventually(t, func() bool { return first.conn.waiters.Load() == 1 }, time.Second, time.Millisecond)
			age := openAIWSConnRotateAge
			if strict {
				age = openAIWSConnMaxAge
			}
			first.conn.createdAtNano.Store(time.Now().Add(-age).UnixNano())
			first.Release()
			result := <-done
			if strict {
				require.ErrorIs(t, result.err, errOpenAIWSPreferredConnUnavailable)
			} else {
				require.NoError(t, result.err)
				require.NotEqual(t, first.ConnID(), result.lease.ConnID(), "waiting must not allow a stale candidate to escape the age policy")
				result.lease.Release()
			}
			require.Zero(t, first.conn.waiters.Load())
		})
	}
}

func TestOpenAIWSConnPool_LifetimeDoesNotStealBusyOrPinnedCapacity(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprint(pinned), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			dialer := &openAIWSCountingDialer{}
			pool.setClientDialerForTest(dialer)
			req := openAIWSAcquireRequest{Account: &Account{ID: 993, Type: AccountTypeAPIKey}, WSURL: "wss://example.com/responses"}
			first, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			first.conn.createdAtNano.Store(time.Now().Add(-openAIWSConnRotateAge).UnixNano())
			if pinned {
				require.True(t, pool.PinConn(req.Account.ID, first.ConnID()))
				first.Release()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			_, err = pool.Acquire(ctx, req)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, 1, dialer.DialCount(), "full pool must wait without exceeding capacity")
			require.False(t, first.conn.isClosed(), "soft rotation must not interrupt another session")
			if pinned {
				pool.UnpinConn(req.Account.ID, first.ConnID())
			} else {
				first.Release()
			}
			next, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			require.NotEqual(t, first.ConnID(), next.ConnID())
			next.Release()
		})
	}
}

// This fixture keeps the upstream reader alive between turns, like the real WS client.
type openAIWSLifetimeReaderConn struct {
	*openAIWSReaderLoopFakeConn
	writes chan []byte
}

func (c *openAIWSLifetimeReaderConn) WriteJSON(_ context.Context, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.writes <- body
	return nil
}

func newOpenAIWSLifetimeSession(t *testing.T, conns []openAIWSClientConn, hooks *OpenAIWSIngressHooks) (*coderws.Conn, *openAIWSConnPool, *openAIWSQueueDialer, <-chan error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(cfg)
	t.Cleanup(pool.Close)
	dialer := &openAIWSQueueDialer{conns: conns}
	pool.setClientDialerForTest(dialer)
	svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector()}
	account := &Account{ID: 994, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, first, err := conn.Read(r.Context())
		if err != nil {
			done <- err
			return
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		done <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "test", first, hooks)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.CloseNow() })
	return client, pool, dialer, done
}

func TestProxyResponsesWebSocket_LifetimeRotatesCompleteHistoryWithReaderLoop(t *testing.T) {
	first := &openAIWSLifetimeReaderConn{openAIWSReaderLoopFakeConn: newOpenAIWSReaderLoopFakeConn(), writes: make(chan []byte, 3)}
	second := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_2","output":[],"usage":{"input_tokens":3,"output_tokens":1}}}`)}}
	var before, after atomic.Int32
	turnDone := make(chan struct{}, 3)
	hooks := &OpenAIWSIngressHooks{BeforeTurn: func(int) error { before.Add(1); return nil }, AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) { after.Add(1); turnDone <- struct{}{} }}
	client, pool, dialer, done := newOpenAIWSLifetimeSession(t, []openAIWSClientConn{first, second}, hooks)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"role":"user","content":"remember alpha"}]}`)))
	<-first.writes
	first.messages <- []byte(`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"cipher","summary":[]},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"alpha saved"}]},{"type":"function_call","call_id":"call_alpha","name":"read","arguments":"{}"}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
	_, _, err := client.Read(ctx)
	require.NoError(t, err)
	<-turnDone
	ap := pool.getOrCreateAccountPool(994)
	ap.mu.Lock()
	var old *openAIWSConn
	for _, conn := range ap.conns {
		old = conn
		conn.createdAtNano.Store(time.Now().Add(-55 * time.Minute).UnixNano())
	}
	ap.mu.Unlock()
	require.NotNil(t, old)
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"previous_response_id":"resp_1","input":[{"role":"user","content":"what was saved?"},{"type":"function_call_output","call_id":"call_alpha","output":"alpha"}]}`)))
	_, response, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "resp_2", gjson.GetBytes(response, "response.id").String())
	<-turnDone
	second.mu.Lock()
	payload := payloadAsJSONBytes(second.lastWrite)
	second.mu.Unlock()
	require.False(t, gjson.GetBytes(payload, "previous_response_id").Exists())
	require.Len(t, gjson.GetBytes(payload, "input").Array(), 6, "both user turns, assistant answer, reasoning and paired tools must survive rotation")
	require.True(t, AnalyzeToolCallOutputContextCoverageBytes(payload).ContextCoversAllCallIDs)
	require.Contains(t, string(payload), "alpha saved")
	require.Contains(t, string(payload), "cipher")
	require.Equal(t, 2, dialer.DialCount())
	require.Equal(t, int32(2), before.Load())
	require.Equal(t, int32(2), after.Load())
	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	require.NoError(t, <-done)
	ap.mu.Lock()
	defer ap.mu.Unlock()
	require.Empty(t, ap.pinnedConns)
	for _, conn := range ap.conns {
		require.False(t, conn.isLeased())
	}
}

const openAIWSLifetimeEvent = `{"type":"error","error":{"code":"websocket_connection_limit_reached","type":"server_error","message":"60 minute maximum websocket lifetime reached"}}`
const openAIWSLifetimeCompleted = `{"type":"response.completed","response":{"id":"resp_recovered","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`

func TestProxyResponsesWebSocket_LifetimeRecoveryBoundaries(t *testing.T) {
	for _, scenario := range []string{"root", "paired_tools", "orphan_tool", "after_output", "twice", "stored_anchor_rejected"} {
		t.Run(scenario, func(t *testing.T) {
			firstEvents := [][]byte{[]byte(openAIWSLifetimeEvent)}
			if scenario == "after_output" {
				firstEvents = append([][]byte{[]byte(`{"type":"response.created","response":{"id":"resp_started"}}`)}, firstEvents...)
			}
			secondEvent := openAIWSLifetimeCompleted
			if scenario == "twice" {
				secondEvent = openAIWSLifetimeEvent
			}
			if scenario == "stored_anchor_rejected" {
				secondEvent = `{"type":"error","error":{"code":"previous_response_not_found","message":"anchor unavailable"}}`
			}
			first := &openAIWSCaptureConn{events: firstEvents}
			second := &openAIWSCaptureConn{events: [][]byte{[]byte(secondEvent)}}
			var before, after atomic.Int32
			hooks := &OpenAIWSIngressHooks{BeforeTurn: func(int) error { before.Add(1); return nil }, AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) { after.Add(1) }}
			client, _, dialer, done := newOpenAIWSLifetimeSession(t, []openAIWSClientConn{first, second}, hooks)
			payload := `{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"role":"user","content":"hello"}]}`
			if scenario == "paired_tools" {
				payload = `{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`
			}
			if scenario == "orphan_tool" {
				payload = `{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`
			}
			if scenario == "stored_anchor_rejected" {
				payload = `{"type":"response.create","model":"gpt-5.1","store":true,"previous_response_id":"resp_external","input":[{"role":"user","content":"next"}]}`
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(payload)))
			if scenario == "root" || scenario == "paired_tools" {
				_, response, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "resp_recovered", gjson.GetBytes(response, "response.id").String(), "lifetime error before output must be hidden by a successful single retry")
				require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
				require.NoError(t, <-done)
			} else {
				if scenario == "after_output" {
					_, created, err := client.Read(ctx)
					require.NoError(t, err)
					require.Equal(t, "response.created", gjson.GetBytes(created, "type").String())
					_, terminal, err := client.Read(ctx)
					require.NoError(t, err)
					require.Equal(t, "error", gjson.GetBytes(terminal, "type").String())
				}
				// Keep reading like the CLI so the gateway can complete its close
				// handshake and join the interrupt reader before returning.
				for {
					_, _, closeErr := client.Read(ctx)
					if closeErr != nil {
						require.NoError(t, ctx.Err(), "terminal failure must close before the session deadline")
						if scenario == "after_output" {
							require.Equal(t, coderws.StatusInternalError, coderws.CloseStatus(closeErr))
						}
						break
					}
				}

				select {
				case err := <-done:
					require.Error(t, err)
				case <-ctx.Done():
					t.Fatal("lifetime recovery did not terminate")
				}
			}
			wantDials := 2
			if scenario == "after_output" || scenario == "orphan_tool" {
				wantDials = 1
			}
			require.Equal(t, wantDials, dialer.DialCount(), "all recovery causes share the one-retry turn budget")
			require.Equal(t, int32(1), before.Load())
			require.Equal(t, int32(1), after.Load())
			if scenario == "stored_anchor_rejected" {
				second.mu.Lock()
				defer second.mu.Unlock()
				require.Equal(t, "resp_external", second.lastWrite["previous_response_id"], "replacement must preserve an external stored anchor")
			}
		})
	}
}

func TestProxyResponsesWebSocket_LifetimeIncompleteHistory(t *testing.T) {
	for _, scenario := range []string{"external_anchor", "missing_output", "reasoning_without_cipher", "unpaired_call"} {
		for _, age := range []time.Duration{55 * time.Minute, 60 * time.Minute} {
			t.Run(scenario+"/"+age.String(), func(t *testing.T) {
				first := &openAIWSLifetimeReaderConn{openAIWSReaderLoopFakeConn: newOpenAIWSReaderLoopFakeConn(), writes: make(chan []byte, 3)}
				turnDone := make(chan struct{}, 3)
				client, pool, dialer, done := newOpenAIWSLifetimeSession(t, []openAIWSClientConn{first}, &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) { turnDone <- struct{}{} }})
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				defer cancel()
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"role":"user","content":"root"}]}`)))
				<-first.writes
				firstOutput := `{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":1}}}`
				switch scenario {
				case "external_anchor":
					firstOutput = `{"type":"response.completed","response":{"id":"resp_1","output":[]}}`
				case "reasoning_without_cipher":
					firstOutput = `{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"reasoning","id":"rs_1","summary":[]}]}}`
				case "unpaired_call":
					firstOutput = `{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"function_call","call_id":"pending","name":"read","arguments":"{}"}]}}`
				}
				first.messages <- []byte(firstOutput)
				_, _, err := client.Read(ctx)
				require.NoError(t, err)
				<-turnDone
				ap := pool.getOrCreateAccountPool(994)
				ap.mu.Lock()
				for _, conn := range ap.conns {
					conn.createdAtNano.Store(time.Now().Add(-age).UnixNano())
				}
				ap.mu.Unlock()
				anchor := "resp_1"
				if scenario == "external_anchor" {
					anchor = "resp_external"
				}
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(fmt.Sprintf(`{"type":"response.create","model":"gpt-5.1","store":false,"previous_response_id":%q,"input":[{"role":"user","content":"next"}]}`, anchor))))
				if age < time.Hour {
					select {
					case payload := <-first.writes:
						require.Equal(t, anchor, gjson.GetBytes(payload, "previous_response_id").String())
					case <-ctx.Done():
						t.Fatal("strict continuation was interrupted before hard expiry")
					}
					first.messages <- []byte(openAIWSLifetimeCompleted)
					_, _, err := client.Read(ctx)
					require.NoError(t, err)
					<-turnDone
					require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
					require.NoError(t, <-done)
				} else {
					select {
					case err := <-done:
						var closeErr *OpenAIWSClientCloseError
						require.ErrorAs(t, err, &closeErr)
					case <-ctx.Done():
						t.Fatal("hard expiry did not fail explicitly")
					}
					require.Empty(t, first.writes, "hard-expired connection must receive no create")
				}
				require.Equal(t, 1, dialer.DialCount())
			})
		}
	}
}

func TestOpenAIWSRetryTotalBudgetIncludesFirstAttempt(t *testing.T) {
	for _, budget := range []time.Duration{0, 5 * time.Second, 30 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			require.Equal(t, budget != 5*time.Second, openAIWSRetryBudgetAllows(15915*time.Millisecond, 120*time.Millisecond, budget))
			require.Equal(t, budget == 0, openAIWSRetryBudgetAllows(31*time.Second, 0, budget), "budget never restarts at first failure")
		})
	}
}

func TestOpenAIWSReplayCollectorLifetimeCompleteness(t *testing.T) {
	for _, fixture := range []struct {
		event    string
		complete bool
	}{
		{`{"type":"response.output_item.done","item":{"type":"message","id":"msg_1","role":"assistant","content":[]}}`, false},
		{`{"type":"response.completed","response":{"id":"resp_1"}}`, false},
		{`{"type":"response.completed","response":{"output":[{}]}}`, false},
		{`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1","summary":[]}]}}`, false},
		{`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"cipher","summary":[]}]}}`, true},
		{`{"type":"response.completed","response":{"output":[{"type":"web_search_call","id":"ws_1","status":"completed"}]}}`, false},
		{`{"type":"response.completed","response":{"output":[]}}`, true},
		{`{"type":"response.completed","response":{"output":[{"type":"message","id":"msg_1","role":"assistant","content":[]}]}}`, true},
	} {
		collector := &openAIWSToolCallReplayCollector{}
		collector.AddEvent(gjson.Get(fixture.event, "type").String(), []byte(fixture.event))
		require.Equal(t, fixture.complete, collector.complete, fixture.event)
	}
	collector := &openAIWSToolCallReplayCollector{}
	collector.AddEvent("response.output_item.done", []byte(`{"item":{"type":"message","id":"msg_missing","role":"assistant","content":[]}}`))
	collector.AddEvent("response.completed", []byte(`{"response":{"output":[]}}`))
	require.False(t, collector.complete, "terminal output must not omit an already observed item")
	require.Len(t, collector.AllItems(), 1, "incomplete terminal output must preserve the collector's existing history")

}

func TestOpenAIGatewayService_WSv2LifetimeRecovery(t *testing.T) {
	for _, scenario := range []string{"root", "stored_anchor_rejected", "invalid_content_after_lifetime", "after_output", "orphan_tool", "small_budget", "large_budget"} {
		t.Run(scenario, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = 30000
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			firstEvents := [][]byte{[]byte(openAIWSLifetimeEvent)}
			if scenario == "after_output" {
				firstEvents = append([][]byte{[]byte(`{"type":"response.output_text.delta","delta":"visible once"}`)}, firstEvents...)
			}
			first := &openAIWSCaptureConn{events: firstEvents}
			if scenario == "small_budget" || scenario == "large_budget" {
				first.readDelays = []time.Duration{150 * time.Millisecond}
				cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = 1000
			}
			if scenario == "small_budget" {
				cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = 50
			}
			nextEvent := openAIWSLifetimeCompleted
			if scenario == "stored_anchor_rejected" {
				nextEvent = `{"type":"error","error":{"code":"previous_response_not_found","message":"anchor unavailable"}}`
			}
			if scenario == "invalid_content_after_lifetime" {
				nextEvent = `{"type":"error","error":{"code":"invalid_encrypted_content","message":"cipher rejected"}}`
			}
			second := &openAIWSCaptureConn{events: [][]byte{[]byte(nextEvent)}}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			pool.setClientDialerForTest(dialer)
			httpUpstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), httpUpstream: httpUpstream}
			account := &Account{ID: 995, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"role":"user","content":"hello"}]}`)
			if scenario == "stored_anchor_rejected" || scenario == "invalid_content_after_lifetime" {
				body = []byte(`{"model":"gpt-5.1","stream":false,"store":true,"previous_response_id":"resp_external","input":[{"type":"reasoning","encrypted_content":"cipher","summary":[]},{"role":"user","content":"hello"}]}`)
			}
			if scenario == "after_output" {
				body = []byte(`{"model":"gpt-5.1","stream":true,"input":[{"role":"user","content":"hello"}]}`)
			}
			if scenario == "orphan_tool" {
				body = []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"function_call_output","call_id":"missing","output":"orphan"}]}`)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := svc.Forward(ctx, c, account, body)
			if scenario == "root" || scenario == "large_budget" {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
			wantDials := 2
			if scenario == "after_output" || scenario == "orphan_tool" || scenario == "small_budget" {
				wantDials = 1
			}
			require.Equal(t, wantDials, dialer.DialCount())
			require.Nil(t, httpUpstream.lastReq, "WS recovery must not fall back to HTTP")
			if scenario == "stored_anchor_rejected" || scenario == "invalid_content_after_lifetime" {
				second.mu.Lock()
				require.Equal(t, "resp_external", second.lastWrite["previous_response_id"])
				second.mu.Unlock()
			}
			if scenario == "after_output" {
				require.Equal(t, 1, strings.Count(rec.Body.String(), "visible once"))
				require.Equal(t, 1, strings.Count(rec.Body.String(), "websocket_connection_limit_reached"))
			}
		})
	}
}

type openAIWSLifetimeHealthConn struct {
	openAIWSFakeConn
	pooled atomic.Pointer[openAIWSConn]
}

func (c *openAIWSLifetimeHealthConn) Ping(context.Context) error {
	if conn := c.pooled.Load(); conn != nil {
		conn.createdAtNano.Store(time.Now().Add(-openAIWSConnRotateAge).UnixNano())
	}
	return nil
}

func TestOpenAIWSConnPool_LifetimeRecheckedAfterHealthCheck(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	t.Cleanup(pool.Close)
	oldClient := &openAIWSLifetimeHealthConn{}
	dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{oldClient, &openAIWSFakeConn{}}}
	pool.setClientDialerForTest(dialer)
	req := openAIWSAcquireRequest{Account: &Account{ID: 996, Type: AccountTypeAPIKey}, WSURL: "wss://example.com/responses"}
	lease, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	old := lease.conn
	oldClient.pooled.Store(old)
	old.createdAtNano.Store(time.Now().Add(-54 * time.Minute).UnixNano())
	lease.Release()
	old.lastUsedNano.Store(time.Now().Add(-2 * time.Minute).UnixNano())
	next, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	defer next.Release()
	require.NotSame(t, old, next.conn, "a candidate can cross the threshold after its token has already been acquired")
	require.Equal(t, 2, dialer.DialCount())
	require.True(t, old.isClosed())
}

func TestProxyResponsesWebSocket_LifetimeRotationRespectsContextWindow(t *testing.T) {
	first := &openAIWSLifetimeReaderConn{openAIWSReaderLoopFakeConn: newOpenAIWSReaderLoopFakeConn(), writes: make(chan []byte, 3)}
	second := &openAIWSCaptureConn{events: [][]byte{[]byte(openAIWSLifetimeCompleted)}}
	turnDone := make(chan struct{}, 2)
	client, pool, _, done := newOpenAIWSLifetimeSession(t, []openAIWSClientConn{first, second}, &OpenAIWSIngressHooks{AfterTurn: func(_ int, _ *OpenAIForwardResult, _ error) { turnDone <- struct{}{} }})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"client_metadata":{"x-codex-window-id":"window_a"},"input":[{"role":"user","content":"old window"}]}`)))
	<-first.writes
	first.messages <- []byte(`{"type":"response.completed","response":{"id":"resp_old_window","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old answer"}]}]}}`)
	_, _, err := client.Read(ctx)
	require.NoError(t, err)
	<-turnDone
	ap := pool.getOrCreateAccountPool(994)
	ap.mu.Lock()
	for _, conn := range ap.conns {
		conn.createdAtNano.Store(time.Now().Add(-55 * time.Minute).UnixNano())
	}
	ap.mu.Unlock()
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","store":false,"previous_response_id":"resp_old_window","client_metadata":{"x-codex-window-id":"window_b"},"input":[{"role":"user","content":"new window"}]}`)))
	_, _, err = client.Read(ctx)
	require.NoError(t, err)
	<-turnDone
	second.mu.Lock()
	payload := payloadAsJSONBytes(second.lastWrite)
	second.mu.Unlock()
	require.False(t, gjson.GetBytes(payload, "previous_response_id").Exists())
	require.Len(t, gjson.GetBytes(payload, "input").Array(), 1, "window rollover must not append the previous window's history")
	require.NotContains(t, string(payload), "old answer")
	require.Equal(t, "new window", gjson.GetBytes(payload, "input.0.content").String())
	require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
	require.NoError(t, <-done)
}
