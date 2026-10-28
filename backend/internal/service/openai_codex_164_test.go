package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodex164PrefixSurvivesFreshHTTPRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			account := &Account{ID: 164, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "fake-local-token", "chatgpt_account_id": "fake-account-164"},
				Extra:       map[string]any{"openai_passthrough": passthrough}}
			capture := func(previous, thread, upstreamAccount string) []string {
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","instructions":"Stable <guidance>","tools":[{"type":"function","name":"local_tool","parameters":{"type":"object","properties":{}}}],"input":[{"role":"user","content":"first question"}],"previous_response_id":%q,"prompt_cache_key":%q}`, previous, thread))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Request.Header.Set(responsesLiteHeader, "true")
				account.Credentials["chatgpt_account_id"] = upstreamAccount
				upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				_, err := svc.Forward(context.Background(), c, account, body)
				require.Error(t, err)
				require.NotEmpty(t, upstream.lastBody)
				require.False(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
				var prefix []string
				for _, item := range gjson.GetBytes(upstream.lastBody, "input").Array() {
					if item.Get("role").String() == "user" {
						break
					}
					require.NotEmpty(t, item.Get("id").String())
					prefix = append(prefix, item.Raw)
				}
				require.Len(t, prefix, 2, "tools and instructions form the reusable prefix")
				return prefix
			}
			first := capture("", "stable-thread", "fake-account-164")
			for _, previous := range []string{"", "resp_first", "resp_second"} {
				require.Equal(t, first, capture(previous, "stable-thread", "fake-account-164"), "fresh HTTP requests and changed response IDs must reuse the prefix within the same caller session")
			}
			require.NotEqual(t, first, capture("", "another-thread", "fake-account-164"))
			require.NotEqual(t, first, capture("", "stable-thread", "fake-account-165"), "account failover must keep credential namespaces isolated")
			require.Equal(t, first, capture("resp_second", "stable-thread", "fake-account-164"), "returning to the original account must not allocate fresh prefix IDs")
		})
	}
}

func TestCodex164RequestPrefix(t *testing.T) {
	account := &Account{ID: 164, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, input := range []string{`"hello"`, `[{"role":"user","content":"hello"}]`} {
		body := []byte(`{"model":"gpt-5.6-sol","instructions":"literal <guidance>","input":` + input + `}`)
		headers := http.Header{"Thread-Id": {"thread-test"}}
		updated, changed, err := normalizeCodexRequestMetadata(body, headers, account, false)
		require.NoError(t, err)
		require.True(t, changed)
		require.False(t, gjson.GetBytes(updated, "instructions").Exists())
		require.Equal(t, "developer", gjson.GetBytes(updated, "input.0.role").String())
		require.Equal(t, "literal <guidance>", gjson.GetBytes(updated, "input.0.content.0.text").String())
		thread := gjson.GetBytes(updated, "client_metadata.thread_id").String()
		namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(thread))
		require.Equal(t, "msg_"+uuid.NewSHA1(namespace, []byte("literal <guidance>")).String(), gjson.GetBytes(updated, "input.0.id").String())
		repeated, changed, err := normalizeCodexRequestMetadata(updated, headers, account, false)
		require.NoError(t, err)
		require.False(t, changed, "a retry must reuse the same prefix and metadata")
		require.Equal(t, updated, repeated)
	}
	for _, body := range []string{
		`{"instructions":"","input":"hello"}`,
		`{"instructions":null,"input":[]}`,
	} {
		updated, _, err := normalizeCodexRequestPrefix([]byte(body), "thread-test")
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(updated, "instructions").Exists())
		require.Equal(t, gjson.Get(body, "input").Raw, gjson.GetBytes(updated, "input").Raw)
	}
}

func TestCodex164ToolPrefixIDUsesSentBytes(t *testing.T) {
	payload := map[string]any{}
	require.NoError(t, decodeOpenAIJSONUseNumber([]byte(`{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"inspect","description":"<tag> & café"}]}],"client_metadata":{"thread_id":"thread-test"}}`), &payload))
	require.NoError(t, normalizeCodexRequestMetadataMap(payload, nil, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	wire, err := marshalOpenAIUpstreamJSON(payload)
	require.NoError(t, err)
	tools := gjson.GetBytes(wire, "input.0.tools").Raw
	require.Contains(t, tools, "<tag> & café")
	namespace := uuid.NewSHA1(uuid.NameSpaceOID, []byte(gjson.GetBytes(wire, "client_metadata.thread_id").String()))
	require.Equal(t, "at_"+uuid.NewSHA1(namespace, []byte(tools)).String(), gjson.GetBytes(wire, "input.0.id").String(), "prefix identity must describe the serialized tools, without Go HTML escaping")
}

func TestCodex164ServerRetryAdvice(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"nested takes precedence", `{"error":{"headers":{"retry-after":"2"}},"headers":{"Retry-After":"9"}}`, "2"},
		{"invalid nested falls through", `{"error":{"headers":{"Retry-After":"-1"}},"headers":{"Retry-After":"3"}}`, "3"},
		{"response failed", `{"response":{"error":{"headers":{"RETRY-AFTER":"4"}}}}`, "4"},
		{"zero is advice", `{"error":{"headers":{"retry-after":"0"}}}`, "0"},
		{"invalid uses HTTP header", `{"error":{"headers":{"retry-after":"1.5"}}}`, "6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{"Retry-After": {"6"}}
			raw, deadline := openAIUpstreamRetryAfter(headers, []byte(tc.body))
			require.Equal(t, tc.want, raw)
			require.False(t, deadline.IsZero())
			failover := newOpenAIUpstreamFailoverError(503, headers, []byte(tc.body), "server_is_overloaded", true)
			require.Equal(t, tc.want, failover.ResponseHeaders.Get("Retry-After"))
			require.WithinDuration(t, deadline, failover.RetryAfter, time.Second)
			require.Equal(t, "6", headers.Get("Retry-After"), "event advice must not mutate the upgrade headers")
		})
	}
	for _, raw := range []string{time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat), "Thu, 01 Jan 1970 00:00:00 GMT"} {
		value, deadline := openAIUpstreamRetryAfter(http.Header{"Retry-After": {raw}}, nil)
		require.Equal(t, raw, value)
		require.False(t, deadline.IsZero())
	}
	body := []byte(`{"type":"error","error":{"code":"server_is_overloaded","headers":{"Retry-After":"2"}}}`)
	updated, changed := sanitizeOpenAICapacityShedErrorCodeForClient(body)
	require.False(t, changed, "native capacity retry advice is understood by Codex 0.164")
	require.Equal(t, body, updated)
}

func TestCodex164PreservesNativeIncrementalContext(t *testing.T) {
	body := []byte(`{"previous_response_id":"resp_1","input":[{"type":"additional_tools","id":"at_native","role":"developer","tools":[{"type":"function","name":"new_tool"}]},{"type":"message","id":"msg_native","role":"developer","content":[{"type":"input_text","text":"Updated model instructions"}]},{"type":"function_call_output","call_id":"call_1","output":"ok"},{"type":"message","role":"developer","content":[{"type":"input_text","text":"Removed tools: old_tool"}]},{"type":"message","role":"assistant","content":[],"encrypted_content":"opaque"}],"client_metadata":{"custom":"steer-value","x-codex-turn-metadata":"{\"history_initialization\":\"cold_resume\"}"}}`)
	updated, changed, err := normalizeCodexRequestPrefix(body, "thread")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, updated, "native tool updates, IDs, history and encrypted replay belong to the caller")
	require.True(t, codexInputHasInstructions(body))
	for _, account := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}} {
		updated, changed, err := normalizeCodexRequestMetadata(body, nil, account, false)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, updated)
	}
	legacy := []byte(`{"instructions":"Compact this","input":[]}`)
	updated, changed, err = normalizeCodexRequestMetadata(legacy, nil, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, true)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, legacy, updated)
}

func TestCodex164LiteOmitsEmptyToolCarrier(t *testing.T) {
	body, _, err := normalizeOpenAIResponsesLiteToolsPayload([]byte(`{"tools":[],"instructions":"Guidance","input":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "tools").Exists())
	require.False(t, gjson.GetBytes(body, "instructions").Exists())
	require.Len(t, gjson.GetBytes(body, "input").Array(), 2)
	require.Equal(t, "message", gjson.GetBytes(body, "input.0.type").String())
}

func TestCodex164WSRetryUsesServerAdvice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, budget := range []int{3000, 200} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.RetryTotalBudgetMS = budget
			cfg.Gateway.OpenAIWS.RetryBackoffInitialMS = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 2
			first := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"error","error":{"code":"server_error","message":"busy","headers":{"Retry-After":"1"}}}`)}}
			second := &openAIWSCaptureConn{events: [][]byte{[]byte(openAIWSLifetimeCompleted)}}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector()}
			account := &Account{ID: 164, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fake-local-key"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			started := time.Now()
			result, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.6-sol","stream":false,"input":[{"role":"user","content":"hello"}]}`))
			if budget == 3000 {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.GreaterOrEqual(t, time.Since(started), time.Second, "server advice replaces the one-millisecond gateway backoff")
				require.Equal(t, 2, dialer.DialCount())
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, 1, dialer.DialCount(), "advice outside the budget must not start another request")
				require.Equal(t, "1", rec.Header().Get("Retry-After"), "remaining advice survives HTTP error projection")
			}
		})
	}
}
