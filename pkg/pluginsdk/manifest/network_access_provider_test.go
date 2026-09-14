package manifest_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadNetworkAccessProvider(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.tailscale", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"network_access_provider.v1", "id":"tailscale", "display_name":"Tailscale",
	    "network_access_provider":{"provider":"tailscale","display_name":"Tailscale"}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := manifest.GetCapabilities()[0].GetNetworkAccessProvider()
	if descriptor.GetProvider() != "tailscale" || descriptor.GetDisplayName() != "Tailscale" {
		t.Fatalf("network access descriptor = %#v", descriptor)
	}
}

func TestValidateNetworkAccessProviderRejectsMissingDescriptor(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{Type: "network_access_provider.v1", Id: "overlay"}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected missing network access descriptor to fail")
	}
}

func TestValidateNetworkAccessProviderRejectsBadSlug(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type: "network_access_provider.v1", Id: "overlay",
			NetworkAccessProvider: &pluginv1.NetworkAccessProviderDescriptor{Provider: "Tail Scale"},
		}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected provider slug with spaces and capitals to fail")
	}
}

func TestValidateNetworkAccessProviderRejectsDescriptorOnOtherType(t *testing.T) {
	manifest := &pluginv1.PluginManifest{
		PluginId: "silo.invalid", Version: "1.0.0",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type: "scheduled_task.v1", Id: "task",
			NetworkAccessProvider: &pluginv1.NetworkAccessProviderDescriptor{Provider: "tailscale"},
		}},
	}
	if err := publicmanifest.Validate(manifest); err == nil {
		t.Fatal("expected network access descriptor on a scheduled task to fail")
	}
}
