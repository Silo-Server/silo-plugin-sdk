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
			ImportRatings:    true,
			ExportRatings:    true,
			SupportedMediaTypes: []pluginv1.WatchSyncMediaType{
				pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE,
				pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE,
				pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_SERIES,
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
		!got.GetImportRatings() || !got.GetExportRatings() ||
		len(got.GetAuthMethods()) != 1 || got.GetAuthMethods()[0] != pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_DEVICE_CODE ||
		len(got.GetSupportedMediaTypes()) != 3 ||
		got.GetSupportedMediaTypes()[1] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE ||
		got.GetSupportedMediaTypes()[2] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_SERIES {
		t.Fatalf("decoded descriptor = %#v", got)
	}
}

// A node on an older SDK must still load a descriptor that a newer SDK wrote
// to the shared database: unknown fields and enum names are dropped.
func TestDecodeCapabilityDiscardsUnknownWatchSyncData(t *testing.T) {
	record := convert.CapabilityRecord{
		Type: "watch_sync_provider.v1",
		ID:   "anilist",
		Metadata: map[string]any{
			"watch_sync_provider": map[string]any{
				"auth_methods":          []any{"WATCH_SYNC_AUTH_METHOD_DEVICE_CODE"},
				"export_watched":        true,
				"max_batch_size":        float64(25),
				"supported_media_types": []any{"WATCH_SYNC_MEDIA_TYPE_MOVIE", "WATCH_SYNC_MEDIA_TYPE_FUTURE_KIND", "WATCH_SYNC_MEDIA_TYPE_EPISODE"},
				"import_future_thing":   true,
			},
			"config_schema": []any{map[string]any{"key": "token", "future_option": "x"}},
		},
	}
	decoded, err := convert.DecodeCapability(record)
	if err != nil {
		t.Fatalf("DecodeCapability() error = %v", err)
	}
	got := decoded.GetWatchSyncProvider()
	if got == nil || !got.GetExportWatched() || got.GetMaxBatchSize() != 25 ||
		len(got.GetAuthMethods()) != 1 || got.GetAuthMethods()[0] != pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_DEVICE_CODE {
		t.Fatalf("decoded descriptor = %#v", got)
	}
	types := got.GetSupportedMediaTypes()
	if len(types) != 2 ||
		types[0] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE ||
		types[1] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE {
		t.Fatalf("supported media types = %v, want the known values only", types)
	}
	if len(got.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("unknown data retained: %x", got.ProtoReflect().GetUnknown())
	}
	if schemas := decoded.GetConfigSchema(); len(schemas) != 1 || schemas[0].GetKey() != "token" {
		t.Fatalf("config schema = %#v", schemas)
	}
}
