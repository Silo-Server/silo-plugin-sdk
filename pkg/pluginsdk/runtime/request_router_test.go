package runtime_test

import (
	"context"
	"net"
	"slices"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type recordingRequestRouter struct {
	pluginv1.UnimplementedRequestRouterServer
	seasons chan []int32
}

func (r recordingRequestRouter) Fulfill(_ context.Context, req *pluginv1.FulfillRequest) (*pluginv1.FulfillResponse, error) {
	r.seasons <- req.GetRequest().GetSeasons()
	return &pluginv1.FulfillResponse{}, nil
}

func TestRequestRouterReceivesRequestedSeasons(t *testing.T) {
	router := recordingRequestRouter{seasons: make(chan []int32, 1)}
	plugins := runtime.DefaultPluginSet(runtime.CapabilityServers{Runtime: stubRuntime{}, RequestRouter: router})
	p := plugins[runtime.PluginSetName].(plugin.GRPCPlugin)
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer = %v, want nil", err)
	}
	listener := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	_, err = pluginv1.NewRequestRouterClient(conn).Fulfill(context.Background(), &pluginv1.FulfillRequest{
		CapabilityId: "arr",
		Request:      &pluginv1.RequestDescriptor{MediaType: "series", Title: "Example", Seasons: []int32{0, 4}},
	})
	if err != nil {
		t.Fatalf("Fulfill = %v", err)
	}
	if got := <-router.seasons; !slices.Equal(got, []int32{0, 4}) {
		t.Fatalf("seasons = %v, want [0 4]", got)
	}
}
