package runtime_test

import (
	"context"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// legacyMarkerProvider is a marker server written before reporting existed: it
// serves only the MarkerProvider service.
type legacyMarkerProvider struct {
	pluginv1.UnimplementedMarkerProviderServer
}

func (legacyMarkerProvider) FetchMarkers(context.Context, *pluginv1.FetchMarkersRequest) (*pluginv1.FetchMarkersResponse, error) {
	return &pluginv1.FetchMarkersResponse{}, nil
}

func (legacyMarkerProvider) SubmitMarker(context.Context, *pluginv1.SubmitMarkerRequest) (*pluginv1.SubmitMarkerResponse, error) {
	return &pluginv1.SubmitMarkerResponse{}, nil
}

func (legacyMarkerProvider) GetMarkerProviderStats(context.Context, *pluginv1.GetMarkerProviderStatsRequest) (*pluginv1.MarkerProviderStatsResponse, error) {
	return &pluginv1.MarkerProviderStatsResponse{}, nil
}

// reportingMarkerProvider serves the fetch side and the separate reporting
// service on one type, which is how the SDK expects a provider to add reporting
// without changing the released MarkerProvider interface.
type reportingMarkerProvider struct {
	pluginv1.UnimplementedMarkerProviderServer
	gotRequest *pluginv1.ReportMarkerRequest
}

func (reportingMarkerProvider) FetchMarkers(context.Context, *pluginv1.FetchMarkersRequest) (*pluginv1.FetchMarkersResponse, error) {
	return &pluginv1.FetchMarkersResponse{}, nil
}

func (reportingMarkerProvider) SubmitMarker(context.Context, *pluginv1.SubmitMarkerRequest) (*pluginv1.SubmitMarkerResponse, error) {
	return &pluginv1.SubmitMarkerResponse{}, nil
}

func (reportingMarkerProvider) GetMarkerProviderStats(context.Context, *pluginv1.GetMarkerProviderStatsRequest) (*pluginv1.MarkerProviderStatsResponse, error) {
	return &pluginv1.MarkerProviderStatsResponse{}, nil
}

func (p *reportingMarkerProvider) ReportMarker(_ context.Context, req *pluginv1.ReportMarkerRequest) (*pluginv1.ReportMarkerResponse, error) {
	p.gotRequest = req
	return &pluginv1.ReportMarkerResponse{
		Outcome: pluginv1.MarkerReportOutcome_MARKER_REPORT_OUTCOME_ALREADY_REPORTED,
	}, nil
}

var (
	_ pluginv1.MarkerProviderServer  = (*legacyMarkerProvider)(nil)
	_ pluginv1.MarkerReportingServer = (*reportingMarkerProvider)(nil)
)

// The report round trip carries the identity the fetch carries, including the
// numbering the host chose, so a provider can address the same episode a second
// time.
func TestMarkerReportingRoundTripWhenProviderImplementsIt(t *testing.T) {
	provider := &reportingMarkerProvider{}
	client := dialPlugin(t, runtime.CapabilityServers{Runtime: stubRuntime{}, MarkerProvider: provider})

	got, err := client.MarkerReporting().ReportMarker(context.Background(), &pluginv1.ReportMarkerRequest{
		ItemType:         "episode",
		ExternalIds:      &pluginv1.MarkerExternalIDs{ImdbId: "tt0903747"},
		SeasonNumber:     5,
		EpisodeNumber:    14,
		Segment:          "intro",
		EpisodeNumbering: "tvdb",
	})
	if err != nil {
		t.Fatalf("ReportMarker = %v", err)
	}
	if got.GetOutcome() != pluginv1.MarkerReportOutcome_MARKER_REPORT_OUTCOME_ALREADY_REPORTED {
		t.Fatalf("outcome = %v, want ALREADY_REPORTED", got.GetOutcome())
	}
	if provider.gotRequest.GetSegment() != "intro" || provider.gotRequest.GetEpisodeNumbering() != "tvdb" ||
		provider.gotRequest.GetSeasonNumber() != 5 || provider.gotRequest.GetEpisodeNumber() != 14 {
		t.Fatalf("request = %v", provider.gotRequest)
	}
}

// A plugin built before the service existed must answer Unimplemented, which
// hosts read as "this provider cannot report" rather than as a failure.
func TestMarkerReportingUnimplementedForLegacyProvider(t *testing.T) {
	client := dialPlugin(t, runtime.CapabilityServers{Runtime: stubRuntime{}, MarkerProvider: legacyMarkerProvider{}})

	_, err := client.MarkerReporting().ReportMarker(context.Background(), &pluginv1.ReportMarkerRequest{Segment: "intro"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("ReportMarker code = %v, want Unimplemented (err %v)", status.Code(err), err)
	}
}

// Reporting lives in its own service, so the released MarkerProvider service
// keeps exactly the three methods plugins already implement.
func TestMarkerProviderServiceGainsNoMethods(t *testing.T) {
	services := registeredServices(t, runtime.CapabilityServers{
		Runtime:        stubRuntime{},
		MarkerProvider: legacyMarkerProvider{},
	})
	info, ok := services["silo.plugin.v1.MarkerProvider"]
	if !ok {
		t.Fatalf("MarkerProvider service not registered; got %v", services)
	}
	want := map[string]bool{"FetchMarkers": true, "SubmitMarker": true, "GetMarkerProviderStats": true}
	if len(info.Methods) != len(want) {
		t.Fatalf("MarkerProvider methods = %v, want %v", info.Methods, want)
	}
	for _, method := range info.Methods {
		if !want[method.Name] {
			t.Fatalf("MarkerProvider gained method %q; report RPCs belong on MarkerReporting", method.Name)
		}
	}
}
