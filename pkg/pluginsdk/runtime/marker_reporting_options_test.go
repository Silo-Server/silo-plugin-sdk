package runtime

import (
	"context"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type optionMarkerProvider struct {
	pluginv1.UnimplementedMarkerProviderServer
}

func (optionMarkerProvider) FetchMarkers(context.Context, *pluginv1.FetchMarkersRequest) (*pluginv1.FetchMarkersResponse, error) {
	return &pluginv1.FetchMarkersResponse{}, nil
}

func (optionMarkerProvider) SubmitMarker(context.Context, *pluginv1.SubmitMarkerRequest) (*pluginv1.SubmitMarkerResponse, error) {
	return &pluginv1.SubmitMarkerResponse{}, nil
}

func (optionMarkerProvider) GetMarkerProviderStats(context.Context, *pluginv1.GetMarkerProviderStatsRequest) (*pluginv1.MarkerProviderStatsResponse, error) {
	return &pluginv1.MarkerProviderStatsResponse{}, nil
}

// optionMarkerReporting is registered through the option rather than baked into
// the provider type, which is how a provider can keep the two services apart.
type optionMarkerReporting struct {
	pluginv1.UnimplementedMarkerReportingServer
}

func (optionMarkerReporting) ReportMarker(_ context.Context, req *pluginv1.ReportMarkerRequest) (*pluginv1.ReportMarkerResponse, error) {
	return &pluginv1.ReportMarkerResponse{
		Outcome: pluginv1.MarkerReportOutcome_MARKER_REPORT_OUTCOME_REPORTED,
		Reason:  req.GetSegment(),
	}, nil
}

func markerManifest() *pluginv1.PluginManifest {
	return &pluginv1.PluginManifest{
		PluginId: "silo.test",
		Version:  "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{
			{Type: capability.MarkerProvider, Id: "test"},
		},
	}
}

func TestManifestServeConfigRegistersMarkerReportingFromOption(t *testing.T) {
	cfg, err := manifestServeConfig(
		markerManifest(),
		CapabilityServers{MarkerProvider: optionMarkerProvider{}},
		WithMarkerReporting(optionMarkerReporting{}),
	)
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}

	got, err := serveConfigClient(t, cfg).MarkerReporting().ReportMarker(context.Background(),
		&pluginv1.ReportMarkerRequest{Segment: "credits"})
	if err != nil {
		t.Fatalf("ReportMarker = %v", err)
	}
	if got.GetOutcome() != pluginv1.MarkerReportOutcome_MARKER_REPORT_OUTCOME_REPORTED || got.GetReason() != "credits" {
		t.Fatalf("response = %v", got)
	}
}

func TestManifestServeConfigSkipsMarkerReportingWithoutAServer(t *testing.T) {
	cfg, err := manifestServeConfig(markerManifest(), CapabilityServers{MarkerProvider: optionMarkerProvider{}})
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}

	_, err = serveConfigClient(t, cfg).MarkerReporting().ReportMarker(context.Background(),
		&pluginv1.ReportMarkerRequest{Segment: "credits"})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("ReportMarker code = %v, want Unimplemented (err %v)", status.Code(err), err)
	}
}
