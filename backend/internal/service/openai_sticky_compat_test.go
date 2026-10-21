package service

import (
	"context"
	"testing"
	"time"

	"github.com/MACOS-DO/sub4api/internal/config"
	"github.com/MACOS-DO/sub4api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStickyTTL_ConfiguredBindingAndLegacyKeys(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     *config.Config
		wantTTL time.Duration
	}{
		{name: "missing_config", wantTTL: time.Hour},
		{name: "partial_config", cfg: &config.Config{}, wantTTL: time.Hour},
		{name: "explicit_1800", cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{StickySessionTTLSeconds: 1800}}}, wantTTL: 30 * time.Minute},
		{name: "explicit_300", cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIWS: config.GatewayOpenAIWSConfig{StickySessionTTLSeconds: 300}}}, wantTTL: 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cfg != nil {
				tc.cfg.Gateway.OpenAIWS.SessionHashDualWriteOld = true
			}
			cache := &stubGatewayCache{}
			svc := &OpenAIGatewayService{cache: cache, cfg: tc.cfg}
			ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
			require.Equal(t, tc.wantTTL, svc.openAIWSSessionStickyTTL())
			require.NoError(t, svc.BindStickySession(ctx, nil, "new-hash", 9))
			legacyTTL := min(tc.wantTTL, 10*time.Minute)
			require.Equal(t, tc.wantTTL, cache.sessionTTLs["openai:new-hash"])
			require.Equal(t, legacyTTL, cache.sessionTTLs["openai:legacy-hash"])

			cache.now = cache.now.Add(time.Minute)
			require.NoError(t, svc.refreshStickySessionTTL(ctx, nil, "new-hash", svc.openAIWSSessionStickyTTL()))
			require.Equal(t, tc.wantTTL, cache.sessionTTLs["openai:new-hash"])
			require.Equal(t, legacyTTL, cache.sessionTTLs["openai:legacy-hash"])
			require.Equal(t, cache.now.Add(tc.wantTTL), cache.sessionExpires["openai:new-hash"])
			require.Equal(t, cache.now.Add(legacyTTL), cache.sessionExpires["openai:legacy-hash"])
		})
	}
}

func TestGetStickySessionAccountID_FallbackToLegacyKey(t *testing.T) {
	beforeFallbackTotal, beforeFallbackHit, _ := openAIStickyCompatStats()

	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:legacy-hash": 42,
		},
	}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashReadOldFallback: true,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	accountID, err := svc.getStickySessionAccountID(ctx, nil, "new-hash")
	require.NoError(t, err)
	require.Equal(t, int64(42), accountID)

	afterFallbackTotal, afterFallbackHit, _ := openAIStickyCompatStats()
	require.Equal(t, beforeFallbackTotal+1, afterFallbackTotal)
	require.Equal(t, beforeFallbackHit+1, afterFallbackHit)
}

func TestSetStickySessionAccountID_DualWriteOldEnabled(t *testing.T) {
	_, _, beforeDualWriteTotal := openAIStickyCompatStats()

	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashDualWriteOld: true,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	err := svc.setStickySessionAccountID(ctx, nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	require.Equal(t, int64(9), cache.sessionBindings["openai:legacy-hash"])

	_, _, afterDualWriteTotal := openAIStickyCompatStats()
	require.Equal(t, beforeDualWriteTotal+1, afterDualWriteTotal)
}

func TestSetStickySessionAccountID_DualWriteOldDisabled(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{
		cache: cache,
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				OpenAIWS: config.GatewayOpenAIWSConfig{
					SessionHashDualWriteOld: false,
				},
			},
		},
	}

	ctx := withOpenAILegacySessionHash(context.Background(), "legacy-hash")
	err := svc.setStickySessionAccountID(ctx, nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	_, exists := cache.sessionBindings["openai:legacy-hash"]
	require.False(t, exists)
}

func TestSnapshotOpenAICompatibilityFallbackMetrics(t *testing.T) {
	before := SnapshotOpenAICompatibilityFallbackMetrics()

	ctx := context.WithValue(context.Background(), ctxkey.ThinkingEnabled, true)
	_, _ = ThinkingEnabledFromContext(ctx)

	after := SnapshotOpenAICompatibilityFallbackMetrics()
	require.GreaterOrEqual(t, after.MetadataLegacyFallbackTotal, before.MetadataLegacyFallbackTotal+1)
	require.GreaterOrEqual(t, after.MetadataLegacyFallbackThinkingEnabledTotal, before.MetadataLegacyFallbackThinkingEnabledTotal+1)
}
