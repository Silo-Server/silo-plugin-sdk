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
			AuthMethods:      []pluginv1.WatchSyncAuthMethod{pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_DEVICE_CODE},
			ExportWatched:    true,
			ImportWatchlist:  true,
			ScrobblePlayback: true,
			SupportedMediaTypes: []pluginv1.WatchSyncMediaType{
				pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE,
				pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE,
			},
			MaxBatchSize: 25,
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
	if got == nil || !got.GetExportWatched() || !got.GetImportWatchlist() || !got.GetScrobblePlayback() || got.GetMaxBatchSize() != 25 ||
		len(got.GetAuthMethods()) != 1 || got.GetAuthMethods()[0] != pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_DEVICE_CODE ||
		len(got.GetSupportedMediaTypes()) != 2 ||
		got.GetSupportedMediaTypes()[1] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE {
		t.Fatalf("decoded descriptor = %#v", got)
	}
}
