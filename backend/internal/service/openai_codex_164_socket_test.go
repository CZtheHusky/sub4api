package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodex164InterruptAndContinuationOnSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ackFirst := range []bool{true, false} {
		name := "completion_before_ack"
		if ackFirst {
			name = "ack_before_completion"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			observed := make(chan []byte, 3)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for frame := 0; frame < 3; frame++ {
					_, body, err := conn.Read(ctx)
					if err != nil {
						return
					}
					observed <- body
					var events []string
					switch frame {
					case 0:
						events = []string{`{"type":"response.created","response":{"id":"resp_1"}}`}
					case 1:
						ack := `{"type":"response.interrupt.accepted","response_id":"resp_1"}`
						completed := `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"message","id":"msg_finished","role":"assistant","content":[{"type":"output_text","text":"finished item"}]}],"usage":{"input_tokens":7,"output_tokens":2}}}`
						events = []string{completed, ack}
						if ackFirst {
							events = []string{ack, completed}
						}
					case 2:
						events = []string{`{"type":"response.completed","response":{"id":"resp_2","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1}}}`}
					}
					for _, event := range events {
						if err := conn.Write(ctx, coderws.MessageText, []byte(event)); err != nil {
							return
						}
					}
				}
				// Let the gateway release the clean connection before closing it.
				_, _, _ = conn.Read(ctx)
			}))
			defer upstream.Close()
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(codexWireSocketDialer{url: "ws" + strings.TrimPrefix(upstream.URL, "http"), inner: newDefaultOpenAIWSClientDialer()})
			defer pool.Close()
			svc := &OpenAIGatewayService{cfg: cfg, openaiWSPool: pool, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector()}
			account := &Account{ID: 164, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "fake-local-token", "chatgpt_account_id": "fake-local-account"},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true}}
			results := make(chan *OpenAIForwardResult, 2)
			done := make(chan error, 1)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				_, first, err := conn.Read(ctx)
				if err != nil {
					done <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "fake-local-token", first,
					&OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, _ error) { results <- result }})
			}))
			defer gateway.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http"), &coderws.DialOptions{HTTPHeader: http.Header{"Session-Id": {"session-164"}, "Thread-Id": {"thread-164"}}})
			require.NoError(t, err)
			defer client.CloseNow()
			read := func() []byte {
				_, body, err := client.Read(ctx)
				require.NoError(t, err)
				return body
			}
			create := `{"type":"response.create","model":"gpt-5.6-terra","stream":true,"input":[{"role":"user","content":"hello"}],"client_metadata":{"custom":"initial","thread_id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8","session_id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8","x-codex-window-id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8:0"}}`
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(create)))
			require.Equal(t, "response.created", gjson.GetBytes(read(), "type").String())
			interrupt := `{"type":"response.interrupt","response_id":"resp_1","mode":"discard_partial_items"}`
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(interrupt)))
			events := []string{gjson.GetBytes(read(), "type").String(), gjson.GetBytes(read(), "type").String()}
			require.ElementsMatch(t, []string{"response.interrupt.accepted", "response.completed"}, events)
			next := `{"type":"response.create","model":"gpt-5.6-terra","stream":true,"previous_response_id":"resp_1","input":[{"type":"additional_tools","id":"at_update","role":"developer","tools":[{"type":"function","name":"new_tool"}]}],"client_metadata":{"custom":"steer","thread_id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8","session_id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8","x-codex-window-id":"019e8f71-1dc8-7f72-9fbc-c3bc6bf549f8:0"}}`
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(next)))
			require.Equal(t, "resp_2", gjson.GetBytes(read(), "response.id").String())
			firstWire := <-observed
			require.False(t, gjson.GetBytes(firstWire, "instructions").Exists())
			require.JSONEq(t, interrupt, string(<-observed), "interrupt must reach the generating connection")
			nextWire := <-observed
			require.Equal(t, "resp_1", gjson.GetBytes(nextWire, "previous_response_id").String())
			require.Equal(t, "steer", gjson.GetBytes(nextWire, "client_metadata.custom").String())
			require.Equal(t, "at_update", gjson.GetBytes(nextWire, "input.0.id").String())
			firstResult := <-results
			require.EqualValues(t, 7, firstResult.Usage.InputTokens)
			require.True(t, firstResult.wsReplayComplete)
			require.Contains(t, string(firstResult.wsReplayInput[0]), "msg_finished")
			require.NotNil(t, <-results)
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			require.NoError(t, <-done)
		})
	}
}
