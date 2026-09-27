package manifest_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadRequestRouterDeclaresSeasonSupport(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"arr", "display_name":"Arr",
	    "request_router":{"supports_seasons":true}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.GetCapabilities()[0].GetRequestRouter().GetSupportsSeasons() {
		t.Fatal("supports_seasons = false, want true")
	}
}

// A manifest written before the descriptor existed stays valid and declares no
// season support, so the host keeps today's whole-series behaviour.
func TestLoadRequestRouterWithoutDescriptorDeclaresNoSeasonSupport(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{"type":"request_router.v1", "id":"arr", "display_name":"Arr"}]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	capability := manifest.GetCapabilities()[0]
	if capability.GetRequestRouter() != nil {
		t.Fatalf("request router descriptor = %#v, want absent", capability.GetRequestRouter())
	}
}

func TestValidateRequestRouterRejectsDescriptorOnOtherType(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type: "scheduled_task.v1", Id: "task",
			RequestRouter: &pluginv1.RequestRouterDescriptor{SupportsSeasons: true},
		}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected request router descriptor on a scheduled task to fail")
	}
}
