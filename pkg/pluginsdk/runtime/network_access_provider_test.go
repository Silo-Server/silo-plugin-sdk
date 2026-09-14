package runtime_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type stubNetworkAccessProvider struct {
	pluginv1.UnimplementedNetworkAccessProviderServer
}

func TestGRPCServerRegistersNetworkAccessProvider(t *testing.T) {
	plugins := runtime.DefaultPluginSet(runtime.CapabilityServers{
		Runtime:               stubRuntime{},
		NetworkAccessProvider: stubNetworkAccessProvider{},
	})
	p, ok := plugins[runtime.PluginSetName].(plugin.GRPCPlugin)
	if !ok {
		t.Fatalf("plugin type = %T, want plugin.GRPCPlugin", plugins[runtime.PluginSetName])
	}
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.NetworkAccessProvider"]; !ok {
		t.Fatalf("NetworkAccessProvider service not registered; got %v", srv.GetServiceInfo())
	}
}

func TestGRPCServerSkipsNilNetworkAccessProvider(t *testing.T) {
	plugins := runtime.DefaultPluginSet(runtime.CapabilityServers{Runtime: stubRuntime{}})
	p := plugins[runtime.PluginSetName].(plugin.GRPCPlugin)
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.NetworkAccessProvider"]; ok {
		t.Fatalf("NetworkAccessProvider registered without a server")
	}
}
