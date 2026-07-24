package manifest_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadWatchSyncProvider(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.anilist", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"watch_sync_provider.v1", "id":"anilist", "display_name":"AniList",
	    "watch_sync_provider":{
	      "auth_methods":["WATCH_SYNC_AUTH_METHOD_AUTHORIZATION_CODE"],
	      "export_watched":true,
	      "supported_media_types":["WATCH_SYNC_MEDIA_TYPE_MOVIE","WATCH_SYNC_MEDIA_TYPE_EPISODE"],
	      "external_id_namespaces":["anilist","tmdb","tvdb"],
	      "max_batch_size":25
	    }
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := manifest.GetCapabilities()[0].GetWatchSyncProvider()
	if descriptor == nil || !descriptor.GetExportWatched() || descriptor.GetMaxBatchSize() != 25 {
		t.Fatalf("watch sync descriptor = %#v", descriptor)
	}
	if got := descriptor.GetSupportedMediaTypes(); len(got) != 2 ||
		got[0] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE ||
		got[1] != pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE {
		t.Fatalf("supported media types = %v", got)
	}
}

func TestValidateWatchSyncProviderRejectsMissingDescriptor(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{Type: "watch_sync_provider.v1", Id: "invalid"}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected missing watch sync descriptor to fail")
	}
}

func TestValidateWatchSyncProviderRejectsUnspecifiedAuthMethod(t *testing.T) {
	manifest := validWatchSyncManifest()
	manifest.Capabilities[0].WatchSyncProvider.AuthMethods = []pluginv1.WatchSyncAuthMethod{
		pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_UNSPECIFIED,
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected unspecified auth method to fail")
	}
}

func TestValidateWatchSyncProviderRejectsEmptyMediaTypes(t *testing.T) {
	manifest := validWatchSyncManifest()
	manifest.Capabilities[0].WatchSyncProvider.SupportedMediaTypes = nil
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected empty supported media types to fail")
	}
}

func TestValidateWatchSyncProviderRejectsUnspecifiedMediaType(t *testing.T) {
	manifest := validWatchSyncManifest()
	manifest.Capabilities[0].WatchSyncProvider.SupportedMediaTypes = []pluginv1.WatchSyncMediaType{
		pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_UNSPECIFIED,
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected unspecified supported media type to fail")
	}
}

func TestValidateWatchSyncProviderAllowsUnknownFutureEnums(t *testing.T) {
	manifest := validWatchSyncManifest()
	manifest.Capabilities[0].WatchSyncProvider.AuthMethods = []pluginv1.WatchSyncAuthMethod{
		pluginv1.WatchSyncAuthMethod(99),
	}
	manifest.Capabilities[0].WatchSyncProvider.SupportedMediaTypes = []pluginv1.WatchSyncMediaType{
		pluginv1.WatchSyncMediaType(99),
	}
	if err := publicmanifest.Validate(manifest); err != nil {
		t.Fatalf("future additive enum values should remain forward compatible: %v", err)
	}
}

func validWatchSyncManifest() *pluginv1.PluginManifest {
	return &pluginv1.PluginManifest{
		PluginId: "silo.valid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type: "watch_sync_provider.v1", Id: "valid",
			WatchSyncProvider: &pluginv1.WatchSyncProviderDescriptor{
				AuthMethods: []pluginv1.WatchSyncAuthMethod{
					pluginv1.WatchSyncAuthMethod_WATCH_SYNC_AUTH_METHOD_API_KEY,
				},
				ExportWatched: true,
				SupportedMediaTypes: []pluginv1.WatchSyncMediaType{
					pluginv1.WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_MOVIE,
				},
				MaxBatchSize: 1,
			},
		}},
	}
}

func TestValidateWatchSyncProviderRejectsDescriptorOnOtherCapability(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type: "event_consumer.v1", Id: "events",
			WatchSyncProvider: &pluginv1.WatchSyncProviderDescriptor{MaxBatchSize: 1},
		}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected misplaced watch sync descriptor to fail")
	}
}
