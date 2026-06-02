package runtime_test

import (
	"context"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"google.golang.org/grpc"
)

// stubRuntime satisfies the required Runtime server by embedding the interface
// (GRPCServer rejects a nil Runtime). Its methods are never called here.
type stubRuntime struct{ pluginv1.RuntimeServer }

type stubScanSource struct{}

func (stubScanSource) PollChanges(context.Context, *pluginv1.PollChangesRequest) (*pluginv1.PollChangesResponse, error) {
	return &pluginv1.PollChangesResponse{}, nil
}

func TestGRPCServerRegistersScanSource(t *testing.T) {
	p := &runtime.GRPCPlugin{Servers: runtime.CapabilityServers{
		Runtime:    stubRuntime{},
		ScanSource: stubScanSource{},
	}}
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer with ScanSource = %v, want nil", err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.ScanSource"]; !ok {
		t.Fatalf("ScanSource service not registered; got %v", srv.GetServiceInfo())
	}
}
