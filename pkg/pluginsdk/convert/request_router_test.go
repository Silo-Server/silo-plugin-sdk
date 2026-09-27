package convert_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

func TestRequestRouterDescriptorRoundTrip(t *testing.T) {
	manifest := &pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
		Type:          "request_router.v1",
		Id:            "arr",
		RequestRouter: &pluginv1.RequestRouterDescriptor{SupportsSeasons: true},
	}}}
	records, err := convert.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.GetRequestRouter().GetSupportsSeasons() {
		t.Fatalf("decoded descriptor = %#v, want supports_seasons", decoded.GetRequestRouter())
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
			"request_router": map[string]any{"supports_seasons": true, "supports_future_thing": true},
		},
	}
	decoded, err := convert.DecodeCapability(record)
	if err != nil {
		t.Fatalf("DecodeCapability() error = %v", err)
	}
	if !decoded.GetRequestRouter().GetSupportsSeasons() {
		t.Fatalf("decoded descriptor = %#v, want supports_seasons", decoded.GetRequestRouter())
	}
}
