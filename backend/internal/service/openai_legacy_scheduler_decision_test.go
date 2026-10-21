//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newLegacySchedulerDecisionTestAccounts(groupID int64, secondWSEnabled bool) []Account {
	return []Account{
		{
			ID: 38101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
			GroupIDs: []int64{groupID},
			Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": true},
		},
		{
			ID: 38102, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 5,
			GroupIDs: []int64{groupID},
			Extra:    map[string]any{"openai_apikey_responses_websockets_v2_enabled": secondWSEnabled},
		},
	}
}

func newLegacySchedulerDecisionTestService(accounts []Account, loadBatch bool, concurrency schedulerTestConcurrencyCache) *OpenAIGatewayService {
	cfg := newSchedulerTestOpenAIWSV2Config()
	cfg.Gateway.Scheduling.LoadBatchEnabled = loadBatch
	return &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(concurrency),
	}
}

func releaseLegacySchedulerDecisionSelection(selection *AccountSelectionResult) {
	if selection != nil && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestLegacySchedulerDecision_StickyTTLRetainsAndRenews(t *testing.T) {
	for _, tc := range []struct {
		name      string
		loadBatch bool
		loadErr   error
	}{
		{name: "priority_lru"},
		{name: "load_batch", loadBatch: true},
		{name: "load_failure", loadBatch: true, loadErr: errors.New("load unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			ctx := context.Background()
			groupID := int64(38200)
			accounts := newLegacySchedulerDecisionTestAccounts(groupID, true)
			svc := newLegacySchedulerDecisionTestService(accounts, tc.loadBatch, schedulerTestConcurrencyCache{loadBatchErr: tc.loadErr})
			// Legacy writes deliberately keep their own TTL even with an explicit configured TTL.
			svc.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds = 1800
			cache := &stubGatewayCache{now: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
			svc.cache = cache
			start := cache.now
			const session = "legacy-ttl-session"
			const key = "openai:" + session
			selectAccount := func(sessionHash string) (*AccountSelectionResult, OpenAIAccountScheduleDecision) {
				selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
					ctx, &groupID, "", sessionHash, "gpt-5.1", nil,
					OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true,
				)
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.NotNil(t, selection.Account)
				require.True(t, selection.Acquired, "TTL checks must not exercise queue rebinding")
				releaseLegacySchedulerDecisionSelection(selection)
				return selection, decision
			}

			selection, decision := selectAccount(session)
			require.Equal(t, int64(38101), selection.Account.ID)
			require.False(t, decision.StickySessionHit)
			createdTTL := cache.sessionTTLs[key]

			accounts[1].Priority = -1
			selection, _ = selectAccount("")
			require.Equal(t, int64(38102), selection.Account.ID, "without a binding the other healthy account must win deterministically")

			cache.now = start.Add(6 * time.Minute)
			selection, decision = selectAccount(session)
			require.Equal(t, int64(38101), selection.Account.ID, "six idle minutes must not discard account affinity")
			require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
			require.True(t, decision.StickySessionHit)
			require.Equal(t, time.Hour, createdTTL, "initial legacy binding must last one hour")
			require.Equal(t, time.Hour, cache.sessionTTLs[key], "sticky hit must refresh for one hour")
			require.Equal(t, cache.now.Add(time.Hour), cache.sessionExpires[key])

			cache.now = start.Add(61 * time.Minute)
			selection, decision = selectAccount(session)
			require.Equal(t, int64(38101), selection.Account.ID, "expiry must follow the refresh, not the initial write")
			require.True(t, decision.StickySessionHit)
			require.Equal(t, cache.now.Add(time.Hour), cache.sessionExpires[key])

			cache.now = cache.now.Add(time.Hour + time.Second)
			selection, decision = selectAccount(session)
			require.Equal(t, int64(38102), selection.Account.ID, "an idle binding must still expire after the last renewal")
			require.False(t, decision.StickySessionHit)
			require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		})
	}
}

func TestLegacySchedulerDecision_StickySessionLayer(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	ctx := context.Background()
	groupID := int64(38100)
	for _, loadBatch := range []bool{true, false} {
		name := "load_batch"
		if !loadBatch {
			name = "priority_lru"
		}
		t.Run(name, func(t *testing.T) {
			svc := newLegacySchedulerDecisionTestService(newLegacySchedulerDecisionTestAccounts(groupID, true), loadBatch, schedulerTestConcurrencyCache{})
			sessionHash := "legacy-sticky-session"
			require.NoError(t, svc.setStickySessionAccountID(ctx, &groupID, sessionHash, 38102, time.Hour))

			selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
				ctx, &groupID, "", sessionHash, "gpt-5.1", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			releaseLegacySchedulerDecisionSelection(selection)
			require.Equal(t, int64(38102), selection.Account.ID, "sticky binding must win over priority order")
			require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
			require.True(t, decision.StickySessionHit)
			require.False(t, decision.StickyPreviousHit)
			require.Equal(t, int64(38102), decision.SelectedAccountID)
			require.Equal(t, AccountTypeAPIKey, decision.SelectedAccountType)

			selection, decision, err = svc.SelectAccountWithSchedulerForCapability(
				ctx, &groupID, "", "", "gpt-5.1", nil,
				OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.NotNil(t, selection.Account)
			releaseLegacySchedulerDecisionSelection(selection)
			require.Equal(t, int64(38101), selection.Account.ID)
			require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
			require.False(t, decision.StickySessionHit)
			require.Equal(t, int64(38101), decision.SelectedAccountID)
		})
	}
}

func TestLegacySchedulerDecision_PreviousResponseRouting(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	ctx := context.Background()
	groupID := int64(38110)
	responseID := "resp_legacy_route"

	t.Run("routes to the account owning the response", func(t *testing.T) {
		svc := newLegacySchedulerDecisionTestService(newLegacySchedulerDecisionTestAccounts(groupID, true), true, schedulerTestConcurrencyCache{})
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "legacy-prev-session", "gpt-5.1", nil,
			OpenAIUpstreamTransportResponsesWebsocketV2Ingress, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38102), selection.Account.ID, "previous_response_id must route to the owning account")
		require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
		require.True(t, decision.StickyPreviousHit)
		require.False(t, decision.StickySessionHit)
		require.Equal(t, int64(38102), decision.SelectedAccountID)
	})

	t.Run("excluded owner falls back to load balance", func(t *testing.T) {
		svc := newLegacySchedulerDecisionTestService(newLegacySchedulerDecisionTestAccounts(groupID, true), true, schedulerTestConcurrencyCache{})
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", map[int64]struct{}{38102: {}},
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38101), selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		require.False(t, decision.StickyPreviousHit)
	})

	t.Run("transport-incompatible owner releases its slot and falls back", func(t *testing.T) {
		released := make([]int64, 0)
		svc := newLegacySchedulerDecisionTestService(newLegacySchedulerDecisionTestAccounts(groupID, false), true, schedulerTestConcurrencyCache{releasedIDs: &released})
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", nil,
			OpenAIUpstreamTransportResponsesWebsocketV2Ingress, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38101), selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		require.False(t, decision.StickyPreviousHit)
		require.Contains(t, released, int64(38102), "the owning account's slot must be released after the transport check fails")
	})

	t.Run("channel-restricted request model fails before routing", func(t *testing.T) {
		released := make([]int64, 0)
		svc := newLegacySchedulerDecisionTestService(newLegacySchedulerDecisionTestAccounts(groupID, true), true, schedulerTestConcurrencyCache{releasedIDs: &released})
		svc.channelService = newTestChannelService(makeStandardRepo(Channel{
			ID:                 38111,
			Status:             StatusActive,
			GroupIDs:           []int64{groupID},
			RestrictModels:     true,
			BillingModelSource: BillingModelSourceChannelMapped,
			ModelPricing:       []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-4o"}}},
			ModelMapping:       map[string]map[string]string{PlatformOpenAI: {"gpt-5.1": "o3-mini"}},
		}, map[int64]string{groupID: PlatformOpenAI}))
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, _, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Contains(t, err.Error(), "channel pricing restriction")
		require.Nil(t, selection)
		require.Empty(t, released, "the owning account's slot must not be touched for a channel-restricted model")
	})

	t.Run("channel-restricted upstream model of the owner releases its slot and falls back", func(t *testing.T) {
		released := make([]int64, 0)
		accounts := newLegacySchedulerDecisionTestAccounts(groupID, true)
		accounts[1].Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.1": "o3-mini"}}
		svc := newLegacySchedulerDecisionTestService(accounts, true, schedulerTestConcurrencyCache{releasedIDs: &released})
		svc.channelService = newTestChannelService(makeStandardRepo(Channel{
			ID:                 38112,
			Status:             StatusActive,
			GroupIDs:           []int64{groupID},
			RestrictModels:     true,
			BillingModelSource: BillingModelSourceUpstream,
			ModelPricing:       []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gpt-5.1"}}},
		}, map[int64]string{groupID: PlatformOpenAI}))
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38101), selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		require.False(t, decision.StickyPreviousHit)
		require.Contains(t, released, int64(38102), "the owning account's slot must be released after the upstream model restriction check fails")
	})

	t.Run("quarantined owner proxy releases its slot and falls back", func(t *testing.T) {
		released := make([]int64, 0)
		healthyProxy, quarantinedProxy := int64(38113), int64(38114)
		accounts := newLegacySchedulerDecisionTestAccounts(groupID, true)
		accounts[0].ProxyID = &healthyProxy
		accounts[1].ProxyID = &quarantinedProxy
		svc := newLegacySchedulerDecisionTestService(accounts, false, schedulerTestConcurrencyCache{releasedIDs: &released})
		svc.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
			failureThreshold: 1,
			failureWindow:    time.Minute,
			quarantineTTL:    10 * time.Minute,
			maxEntries:       16,
		})
		tripped, _ := svc.openaiProxyStreamCircuit.recordFailure(quarantinedProxy, time.Now())
		require.True(t, tripped)
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38101), selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
		require.False(t, decision.StickyPreviousHit)
		require.Contains(t, released, int64(38102), "the owning account's slot must be released after the proxy quarantine check fails")
	})

	t.Run("all proxies quarantined fails open to the owner", func(t *testing.T) {
		proxy := int64(38115)
		accounts := newLegacySchedulerDecisionTestAccounts(groupID, true)
		accounts[0].ProxyID = &proxy
		accounts[1].ProxyID = &proxy
		svc := newLegacySchedulerDecisionTestService(accounts, false, schedulerTestConcurrencyCache{})
		svc.openaiProxyStreamCircuit = newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
			failureThreshold: 1,
			failureWindow:    time.Minute,
			quarantineTTL:    10 * time.Minute,
			maxEntries:       16,
		})
		tripped, _ := svc.openaiProxyStreamCircuit.recordFailure(proxy, time.Now())
		require.True(t, tripped)
		require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, responseID, 38102, time.Hour))

		selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
			ctx, &groupID, responseID, "", "gpt-5.1", nil,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, true, true,
		)
		require.NoError(t, err, "quarantine must fail open instead of returning no available accounts")
		require.NotNil(t, selection)
		require.NotNil(t, selection.Account)
		releaseLegacySchedulerDecisionSelection(selection)
		require.Equal(t, int64(38102), selection.Account.ID)
		require.Equal(t, openAIAccountScheduleLayerPreviousResponse, decision.Layer)
		require.True(t, decision.StickyPreviousHit)
	})
}
