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

// Use the production dialer over a real socket, changing only the destination
// so OAuth requests never contact a provider during this regression.
type codexWireSocketDialer struct {
	url   string
	inner openAIWSClientDialer
}

func TestCodexGuardianRoutingPinOnSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type observed struct {
		dial       int32
		headers    http.Header
		body       []byte
		responseID string
	}
	requests := make(chan observed, 16)
	var dials atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dial := dials.Add(1)
		parentConnection := r.Header.Get(openAISubagentHeader) == ""
		if parentConnection {
			w.Header().Add("Set-Cookie", "__oailb=parent-shard; Path=/backend-api/codex; Secure")
			w.Header().Add("Set-Cookie", "__cflb=parent-edge; Secure")
			w.Header().Add("Set-Cookie", "session=private; Secure")
		}
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			_, body, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			responseID := "resp_other_turn"
			if parentConnection {
				responseID = fmt.Sprintf("resp_parent_%d", dial)
			}
			requests <- observed{dial: dial, headers: r.Header.Clone(), body: body, responseID: responseID}
			for _, event := range []string{"response.created", "response.completed"} {
				payload := `{"type":"` + event + `","response":{"id":"` + responseID + `","output":[],"status":"completed"}}`
				if err := conn.Write(r.Context(), coderws.MessageText, []byte(payload)); err != nil {
					return
				}
			}
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 6
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 6
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(cfg)
	dialer := codexWireSocketDialer{url: "ws" + strings.TrimPrefix(upstream.URL, "http"), inner: newDefaultOpenAIWSClientDialer()}
	pool.setClientDialerForTest(dialer)
	defer pool.Close()
	svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, openaiWSPassthroughDialer: dialer, toolCorrector: NewCodexToolCorrector()}
	account := &Account{ID: 572, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 6, Credentials: map[string]any{"chatgpt_account_id": "wire-account"}}
	forward := func(caller, group int64, token, marker, model, input, parent string) observed {
		t.Helper()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Set("api_key", &APIKey{ID: caller, GroupID: &group})
		c.Request.Header.Set("Cookie", "__oailb=client-forgery; session=client-secret")
		c.Request.Header.Set("thread-id", "parent-thread")
		payload := map[string]any{"model": model, "input": input, "service_tier": "priority"}
		if parent != "" {
			c.Request.Header.Set("x-openai-subagent", "review")
			parentThread := "parent-thread"
			if input == "thread" {
				parentThread = "unrelated-thread"
			}
			c.Request.Header.Set("x-codex-parent-thread-id", parentThread)
			payload["client_metadata"] = map[string]any{"parent_response_id": parent}
		}
		if marker != "" {
			c.Request.Header.Set("x-codex-guardian", marker)
		}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ctx = WithOpenAIGuardianParentAffinity(ctx, c, raw, model)
		result, err := svc.forwardOpenAIWSV2(ctx, c, account, payload, "wire-session", "wire-guardian", token, OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, true, model, model, time.Now(), 1, "", new(bool))
		require.NoError(t, err)
		require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
		select {
		case got := <-requests:
			return got
		case <-ctx.Done():
			t.Fatal("websocket upstream did not see a request")
			return observed{}
		}
	}

	parent := forward(1, 2, "test-token", "", "gpt-5.6-sol", "parent", "")
	require.Empty(t, parent.headers.Get("Cookie"))
	review := forward(1, 2, "test-token", "reviewer", codexAutoReviewModel, "review", parent.responseID)
	require.NotEqual(t, parent.dial, review.dial, "parent and reviewer must have separate handshakes")
	require.Equal(t, "__oailb=parent-shard; __cflb=parent-edge", review.headers.Get("Cookie"))
	require.Empty(t, review.headers.Get("X-Codex-Guardian"), "the gateway must not grant a private upstream reviewer role")
	require.Empty(t, review.headers.Get(openAICodexRoutingHintHeader))
	require.False(t, gjson.GetBytes(review.body, "service_tier").Exists())
	require.Equal(t, parent.responseID, gjson.GetBytes(review.body, "client_metadata.parent_response_id").String())
	secondReview := forward(1, 2, "test-token", "reviewer", codexAutoReviewModel, "review again", parent.responseID)
	require.Equal(t, review.dial, secondReview.dial, "the same parent pin permits a compatible reviewer connection to be reused")

	for _, tc := range []struct {
		name                     string
		caller, group            int64
		token, marker, model, id string
	}{
		{"caller", 2, 2, "test-token", "reviewer", codexAutoReviewModel, parent.responseID},
		{"group", 1, 3, "test-token", "reviewer", codexAutoReviewModel, parent.responseID},
		{"credential", 1, 2, "other-token", "reviewer", codexAutoReviewModel, parent.responseID},
		{"role", 1, 2, "test-token", "async-reviewer", codexAutoReviewModel, parent.responseID},
		{"model", 1, 2, "test-token", "reviewer", "gpt-5.6-sol", parent.responseID},
		{"parent", 1, 2, "test-token", "reviewer", codexAutoReviewModel, "resp_other_turn"},
		{"thread", 1, 2, "test-token", "reviewer", codexAutoReviewModel, parent.responseID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := forward(tc.caller, tc.group, tc.token, tc.marker, tc.model, tc.name, tc.id)
			require.NotEqual(t, review.dial, got.dial)
			require.Empty(t, got.headers.Get("Cookie"))
		})
	}
	ingress := func(t *testing.T, accountID int64, mode, parentID string) observed {
		t.Helper()
		socketAccount := &Account{
			ID: accountID, Platform: account.Platform, Type: account.Type, Concurrency: account.Concurrency,
			Credentials: account.Credentials, Extra: map[string]any{"openai_oauth_responses_websockets_v2_mode": mode},
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := coderws.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			rctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			_, body, err := conn.Read(rctx)
			if err != nil {
				return
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = r
			groupID := int64(2)
			c.Set("api_key", &APIKey{ID: 1, GroupID: &groupID})
			rctx = WithOpenAIGuardianParentAffinity(rctx, c, body, gjson.GetBytes(body, "model").String())
			_ = svc.ProxyResponsesWebSocketFromClient(rctx, c, conn, socketAccount, "test-token", body, nil)
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		headers := http.Header{}
		headers.Set("User-Agent", "codex_cli_rs/0.166.0")
		headers.Set("Thread-Id", "parent-thread")
		headers.Set("Cookie", "__cflb=client-forgery")
		create := map[string]any{
			"type": "response.create", "model": "gpt-5.6-sol", "service_tier": "priority",
			"input": []any{}, "stream": true,
		}
		if parentID != "" {
			headers.Set(openAISubagentHeader, "review")
			headers.Set(codexParentThreadIDHeader, "parent-thread")
			headers.Set("X-Codex-Guardian", "reviewer")
			create["model"] = codexAutoReviewModel
			create["client_metadata"] = map[string]any{"parent_response_id": parentID}
		}
		conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &coderws.DialOptions{HTTPHeader: headers})
		require.NoError(t, err)
		defer conn.CloseNow()
		payload, err := json.Marshal(create)
		require.NoError(t, err)
		require.NoError(t, conn.Write(ctx, coderws.MessageText, payload))
		for {
			_, event, err := conn.Read(ctx)
			require.NoError(t, err, "%s did not relay response.completed", mode)
			if gjson.GetBytes(event, "type").String() == "response.completed" {
				break
			}
		}
		select {
		case got := <-requests:
			return got
		case <-ctx.Done():
			t.Fatal("websocket upstream did not see a request")
			return observed{}
		}
	}
	for i, modes := range [][2]string{
		{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough},
		{OpenAIWSIngressModePassthrough, OpenAIWSIngressModeCtxPool},
	} {
		t.Run(modes[0]+"-to-"+modes[1], func(t *testing.T) {
			accountID := account.ID + int64(i) + 1
			parent := ingress(t, accountID, modes[0], "")
			require.Empty(t, parent.headers.Get("Cookie"))
			review := ingress(t, accountID, modes[1], parent.responseID)
			require.NotEqual(t, parent.dial, review.dial)
			require.Equal(t, "__oailb=parent-shard; __cflb=parent-edge", review.headers.Get("Cookie"))
			require.Empty(t, review.headers.Get(openAICodexRoutingHintHeader))
			require.False(t, gjson.GetBytes(review.body, "service_tier").Exists())
		})
	}
}

func (d codexWireSocketDialer) Dial(ctx context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	return d.inner.Dial(ctx, d.url, headers, "")
}

func TestCodexWSCompatibilityMetadataOnSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type observed struct {
		headers http.Header
		body    []byte
		err     error
	}
	requests := make(chan observed, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			requests <- observed{err: err}
			return
		}
		conn.SetReadLimit(2 << 20)
		for {
			_, body, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			requests <- observed{headers: r.Header.Clone(), body: body}
			if err := conn.Write(r.Context(), coderws.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp_wire","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":1}}}`)); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(codexWireSocketDialer{url: "ws" + strings.TrimPrefix(upstream.URL, "http"), inner: newDefaultOpenAIWSClientDialer()})
	defer pool.Close()
	svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, toolCorrector: NewCodexToolCorrector()}
	account := &Account{ID: 571, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"chatgpt_account_id": "wire-test-account"}}
	metadata := `{"session_id":"wire-session","thread_id":"wire-thread","request_kind":"turn","guardian_recorded_items":true,"tool_namespaces_info":{"tools":{"name":"tools","functions":{"execute":{"name":"execute","direct":true}}}}}`
	inventoryName := strings.Repeat("x", 70<<10)
	metadata = strings.Replace(metadata, `"name":"tools"`, `"name":"`+inventoryName+`"`, 1)
	for range 2 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("session-id", "wire-session")
		c.Request.Header.Set("thread-id", "wire-thread")
		c.Request.Header.Set("x-responsesapi-include-timing-metrics", "true")
		c.Request.Header.Set("x-openai-subagent", "review")
		c.Request.Header.Set("x-openai-memgen-request", "true")
		c.Request.Header.Set("x-oai-attestation", "must-not-forward-client-proof")
		c.Request.Header.Set("Cookie", "test-route=downstream-client-value")
		payload := map[string]any{"model": "gpt-5.6-sol", "input": "hello", "client_metadata": map[string]any{"x-codex-turn-metadata": metadata}}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result, err := svc.forwardOpenAIWSV2(ctx, c, account, payload, "wire-session", "wire-scope", "test-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, true, "gpt-5.6-sol", "gpt-5.6-sol", time.Now(), 1, "", new(bool))
		cancel()
		require.NoError(t, err)
		require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
		got := <-requests
		require.NoError(t, got.err)
		require.Equal(t, "true", got.headers.Get("x-responsesapi-include-timing-metrics"))
		require.Equal(t, "review", got.headers.Get("x-openai-subagent"))
		require.Equal(t, "true", got.headers.Get("x-openai-memgen-request"))
		for _, name := range []string{"session_id", "conversation_id", "x-codex-installation-id", "x-oai-attestation", "Cookie"} {
			require.Empty(t, got.headers.Get(name), name)
		}
		headerMetadata := gjson.Parse(got.headers.Get(openAIWSTurnMetadataHeader))
		require.True(t, strings.HasPrefix(headerMetadata.Raw, `{"guardian_recorded_items":true,"installation_id":`))
		require.False(t, headerMetadata.Get("tool_namespaces_info").Exists(), "unbounded tool inventory must not be in the HTTP upgrade header")
		bodyMetadata := gjson.Parse(gjson.GetBytes(got.body, "client_metadata.x-codex-turn-metadata").String())
		require.True(t, strings.HasPrefix(bodyMetadata.Raw, `{"guardian_recorded_items":true,"installation_id":`))
		require.Equal(t, "execute", bodyMetadata.Get("tool_namespaces_info.tools.functions.execute.name").String())
		require.Equal(t, inventoryName, bodyMetadata.Get("tool_namespaces_info.tools.name").String())
		require.Equal(t, bodyMetadata.Get("thread_id").String(), got.headers.Get("thread-id"))
		require.Equal(t, "response.create", gjson.GetBytes(got.body, "type").String())
		require.Equal(t, []string{"type", "model", "stream"}, openAIUpstreamJSONTopLevelKeys(got.body)[:3], "synthesized WS frames must route before input")
		require.Contains(t, rec.Body.String(), "response.completed")
	}
}
