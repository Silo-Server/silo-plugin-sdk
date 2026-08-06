package pluginv1

import "context"

// legacyDirectWatchSyncProvider is deliberately implemented without embedding
// UnimplementedWatchSyncProviderServer. Keep this compile-time assertion so a
// future additive RPC cannot silently break source compatibility for v0.12
// plugins generated with require_unimplemented_servers=false.
type legacyDirectWatchSyncProvider struct{}

func (legacyDirectWatchSyncProvider) InitAuthorize(context.Context, *WatchSyncInitAuthorizeRequest) (*WatchSyncInitAuthorizeResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) ExchangeCode(context.Context, *WatchSyncExchangeCodeRequest) (*WatchSyncCredentialResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) ExchangeAPIKey(context.Context, *WatchSyncExchangeAPIKeyRequest) (*WatchSyncCredentialResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) RefreshCredentials(context.Context, *WatchSyncRefreshCredentialsRequest) (*WatchSyncCredentialResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) GetAccount(context.Context, *WatchSyncGetAccountRequest) (*WatchSyncGetAccountResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) ApplyEvents(context.Context, *WatchSyncApplyEventsRequest) (*WatchSyncApplyEventsResponse, error) {
	return nil, nil
}

func (legacyDirectWatchSyncProvider) ListRemoteState(context.Context, *WatchSyncListRemoteStateRequest) (*WatchSyncListRemoteStateResponse, error) {
	return nil, nil
}

var _ WatchSyncProviderServer = legacyDirectWatchSyncProvider{}
