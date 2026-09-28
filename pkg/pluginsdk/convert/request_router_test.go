package convert_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

func TestRequestRouterDescriptorRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name       string
		descriptor *pluginv1.RequestRouterDescriptor
	}{
		{name: "seasons", descriptor: &pluginv1.RequestRouterDescriptor{SupportsSeasons: true}},
		{name: "download progress", descriptor: &pluginv1.RequestRouterDescriptor{ReportsDownloadProgress: true}},
		{name: "both", descriptor: &pluginv1.RequestRouterDescriptor{SupportsSeasons: true, ReportsDownloadProgress: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := &pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
				Type:          "request_router.v1",
				Id:            "arr",
				RequestRouter: tc.descriptor,
			}}}
			records, err := convert.CapabilityRecordsFromManifest(manifest)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := convert.DecodeCapability(records[0])
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(decoded.GetRequestRouter(), tc.descriptor) {
				t.Fatalf("decoded descriptor = %v, want %v", decoded.GetRequestRouter(), tc.descriptor)
			}
		})
	}
}

// The host stores capability metadata as JSON and other nodes read it back,
// so the flag is stored under its proto field name.
func TestCapabilityRecordStoresDownloadProgressFlag(t *testing.T) {
	manifest := &pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
		Type:          "request_router.v1",
		Id:            "arr",
		RequestRouter: &pluginv1.RequestRouterDescriptor{ReportsDownloadProgress: true},
	}}}
	records, err := convert.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := records[0].Metadata["request_router"].(map[string]any)
	if stored["reports_download_progress"] != true {
		t.Fatalf("stored request_router = %v, want reports_download_progress true", stored)
	}
}

// Metadata stored for a request router built on an SDK that knew only
// supports_seasons must decode as not reporting download progress.
func TestDecodeCapabilityWithOnlySeasonSupportReportsNoDownloadProgress(t *testing.T) {
	record := convert.CapabilityRecord{
		Type: "request_router.v1",
		ID:   "arr",
		Metadata: map[string]any{
			"request_router": map[string]any{"supports_seasons": true},
		},
	}
	decoded, err := convert.DecodeCapability(record)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.GetRequestRouter().GetReportsDownloadProgress() {
		t.Fatalf("decoded descriptor = %v, want reports_download_progress false", decoded.GetRequestRouter())
	}
}

// Metadata stored for a request router built on an older SDK has no
// request_router key; it must decode as declaring no season support.
func TestDecodeCapabilityWithoutRequestRouterDeclaresNoSeasonSupport(t *testing.T) {
	record := convert.CapabilityRecord{
		Type:     "request_router.v1",
		ID:       "arr",
		Metadata: map[string]any{"display_name": "Arr"},
	}
	decoded, err := convert.DecodeCapability(record)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.GetRequestRouter() != nil {
		t.Fatalf("decoded descriptor = %#v, want absent", decoded.GetRequestRouter())
	}
}

func TestDecodeCapabilityDiscardsUnknownRequestRouterData(t *testing.T) {
	record := convert.CapabilityRecord{
		Type: "request_router.v1",
		ID:   "arr",
		Metadata: map[string]any{
			"request_router": map[string]any{
				"supports_seasons":          true,
				"reports_download_progress": true,
				"supports_future_thing":     true,
			},
		},
	}
	decoded, err := convert.DecodeCapability(record)
	if err != nil {
		t.Fatalf("DecodeCapability() error = %v", err)
	}
	if !decoded.GetRequestRouter().GetSupportsSeasons() || !decoded.GetRequestRouter().GetReportsDownloadProgress() {
		t.Fatalf("decoded descriptor = %#v, want supports_seasons and reports_download_progress", decoded.GetRequestRouter())
	}
}
