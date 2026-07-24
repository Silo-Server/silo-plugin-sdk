package runtime_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"google.golang.org/grpc"
)

type stubWatchSyncProvider struct {
	pluginv1.UnimplementedWatchSyncProviderServer
}

func TestGRPCServerRegistersWatchSyncProvider(t *testing.T) {
	p := &runtime.GRPCPlugin{Servers: runtime.CapabilityServers{
		Runtime:           stubRuntime{},
		WatchSyncProvider: stubWatchSyncProvider{},
	}}
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer with WatchSyncProvider = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.WatchSyncProvider"]; !ok {
		t.Fatalf("WatchSyncProvider service not registered; got %v", srv.GetServiceInfo())
	}
}
