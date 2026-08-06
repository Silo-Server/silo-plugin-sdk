package runtime_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type stubWatchSyncProvider struct {
	pluginv1.UnimplementedWatchSyncProviderServer
}

type stubWatchSyncDeviceAuthorization struct {
	pluginv1.UnimplementedWatchSyncDeviceAuthorizationServiceServer
}

func TestGRPCServerRegistersWatchSyncServices(t *testing.T) {
	plugins := runtime.DefaultPluginSetWithWatchSyncDeviceAuthorization(runtime.CapabilityServers{
		Runtime:           stubRuntime{},
		WatchSyncProvider: stubWatchSyncProvider{},
	}, stubWatchSyncDeviceAuthorization{})
	p, ok := plugins[runtime.PluginSetName].(plugin.GRPCPlugin)
	if !ok {
		t.Fatalf("watch-sync plugin type = %T, want plugin.GRPCPlugin", plugins[runtime.PluginSetName])
	}
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer with watch-sync services = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.WatchSyncProvider"]; !ok {
		t.Fatalf("WatchSyncProvider service not registered; got %v", srv.GetServiceInfo())
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.WatchSyncDeviceAuthorizationService"]; !ok {
		t.Fatalf("WatchSyncDeviceAuthorization service not registered; got %v", srv.GetServiceInfo())
	}
}
