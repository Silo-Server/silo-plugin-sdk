package runtime_test

import (
	"context"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"google.golang.org/grpc"
)

type stubImageResolver struct{}

func (stubImageResolver) ResolveImageURL(context.Context, *pluginv1.ResolveImageURLRequest) (*pluginv1.ResolveImageURLResponse, error) {
	return &pluginv1.ResolveImageURLResponse{}, nil
}

func (stubImageResolver) ResolveImageURLs(context.Context, *pluginv1.ResolveImageURLsRequest) (*pluginv1.ResolveImageURLsResponse, error) {
	return &pluginv1.ResolveImageURLsResponse{}, nil
}

func TestGRPCServerRegistersImageResolver(t *testing.T) {
	p := &runtime.GRPCPlugin{Servers: runtime.CapabilityServers{
		Runtime:       stubRuntime{},
		ImageResolver: stubImageResolver{},
	}}
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer with ImageResolver = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.ImageResolver"]; !ok {
		t.Fatalf("ImageResolver service not registered; got %v", srv.GetServiceInfo())
	}
}
