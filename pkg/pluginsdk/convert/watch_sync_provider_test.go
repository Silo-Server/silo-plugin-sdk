package convert_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

func TestWatchSyncProviderDescriptorRoundTrip(t *testing.T) {
	manifest := &pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
		Type: "watch_sync_provider.v1",
		Id:   "anilist",
		WatchSyncProvider: &pluginv1.WatchSyncProviderDescriptor{
			AuthMethods:         []pluginv1.WatchSyncAuthMethod{pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_AUTHORIZATION_CODE},
			ExportWatched:       true,
			SupportedMediaTypes: []string{"movie", "episode"},
			MaxBatchSize:        25,
		},
	}}}
	records, err := convert.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.GetWatchSyncProvider()
	if got == nil || !got.GetExportWatched() || got.GetMaxBatchSize() != 25 || len(got.GetAuthMethods()) != 1 {
		t.Fatalf("decoded descriptor = %#v", got)
	}
}
