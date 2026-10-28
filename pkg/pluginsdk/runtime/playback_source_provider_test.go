package runtime

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type playbackSourceTestRuntime struct {
	pluginv1.UnimplementedRuntimeServer
}

type playbackSourceTestServer struct {
	pluginv1.UnimplementedPlaybackSourceProviderServer
}

func TestGRPCServerRegistersPlaybackSourceProvider(t *testing.T) {
	plugins := pluginSetWithOptionalServices(
		CapabilityServers{Runtime: playbackSourceTestRuntime{}},
		optionalServices{playbackSourceProvider: playbackSourceTestServer{}},
	)
	p, ok := plugins[PluginSetName].(plugin.GRPCPlugin)
	if !ok {
		t.Fatalf("playback-source plugin type = %T, want plugin.GRPCPlugin", plugins[PluginSetName])
	}
	server := grpc.NewServer()
	if err := p.GRPCServer(nil, server); err != nil {
		t.Fatalf("GRPCServer = %v", err)
	}
	if _, ok := server.GetServiceInfo()["silo.plugin.v1.PlaybackSourceProvider"]; !ok {
		t.Fatalf("PlaybackSourceProvider not registered: %v", server.GetServiceInfo())
	}
}

func TestWithPlaybackSourceProviderWiresOptionalService(t *testing.T) {
	var options serveManifestOptions
	WithPlaybackSourceProvider(playbackSourceTestServer{})(&options)
	if options.playbackSourceProvider == nil {
		t.Fatal("playback source provider option was not wired")
	}
}

func TestClientPlaybackSourceProvider(t *testing.T) {
	if NewClient(nil).PlaybackSourceProvider() == nil {
		t.Fatal("PlaybackSourceProvider client is nil")
	}
}
