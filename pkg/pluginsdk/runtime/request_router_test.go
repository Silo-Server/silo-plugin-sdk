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
	requests chan *pluginv1.RequestDescriptor
}

func (r recordingRequestRouter) Fulfill(_ context.Context, req *pluginv1.FulfillRequest) (*pluginv1.FulfillResponse, error) {
	r.requests <- req.GetRequest()
	return &pluginv1.FulfillResponse{}, nil
}

func TestRequestRouterReceivesRequestedSeasons(t *testing.T) {
	router := recordingRequestRouter{requests: make(chan *pluginv1.RequestDescriptor, 1)}
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
		Request: &pluginv1.RequestDescriptor{MediaType: "series", Title: "Example", Seasons: []int32{0, 4},
			SeasonDetails: []*pluginv1.RequestedSeason{{Number: 4, FirstAirDate: "2024-10-04", EpisodeCount: 24}}},
	})
	if err != nil {
		t.Fatalf("Fulfill = %v", err)
	}
	got := <-router.requests
	if !slices.Equal(got.GetSeasons(), []int32{0, 4}) {
		t.Fatalf("seasons = %v, want [0 4]", got.GetSeasons())
	}
	details := got.GetSeasonDetails()
	if len(details) != 1 || details[0].GetNumber() != 4 || details[0].GetFirstAirDate() != "2024-10-04" || details[0].GetEpisodeCount() != 24 {
		t.Fatalf("season details = %v, want season 4 from 2024-10-04 with 24 episodes", details)
	}

	// A host that predates season_details sends only the numbers.
	if _, err = pluginv1.NewRequestRouterClient(conn).Fulfill(context.Background(), &pluginv1.FulfillRequest{
		CapabilityId: "arr",
		Request:      &pluginv1.RequestDescriptor{MediaType: "series", Title: "Example", Seasons: []int32{2}},
	}); err != nil {
		t.Fatalf("Fulfill without details = %v", err)
	}
	if got := <-router.requests; !slices.Equal(got.GetSeasons(), []int32{2}) || len(got.GetSeasonDetails()) != 0 {
		t.Fatalf("seasons = %v, details = %v; want [2] and none", got.GetSeasons(), got.GetSeasonDetails())
	}
}
